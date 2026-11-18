package ble

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// The Lumix Sync GATT profile, as reverse-engineered from a LUMIX S5 by
// github.com/tobiasbrummer/lux-lat-long-log. Panasonic gives its services
// 128-bit UUIDs under the suffix -0002a5d5c51b; the UUIDs are stable across
// connections, unlike the handles.
//
// The camera needs no pairing or bonding: its ATT writes are plaintext, and
// the "pairing" the app does is application-layer (the MEI0 handshake below).
const (
	// Session and device names.
	uuidSession  = "8d08a420-3213-11e6-8aca-0002a5d5c51b" // MEI0, the handshake
	uuidDevName  = "cd7a71a0-3213-11e6-8f56-0002a5d5c51b" // this client's name
	uuidClock    = "ead55e60-3216-11e6-a42e-0002a5d5c51b" // the camera's clock
	uuidGPSReady = "37701b80-32a7-11e6-bb83-0002a5d5c51b" // notifies 0x01
	uuidClkReady = "4cf487c0-32a7-11e6-a50d-0002a5d5c51b" // notifies 0x01

	// The Location service, which a Lumix advertises: the fastest way to
	// tell a camera from the other devices in the room.
	ServiceLocation = "1d74afe0-3214-11e6-8ab4-0002a5d5c51b"
)

// mei0 is the fixed session packet. The camera does not check its counter or
// checksum strictly, so one captured value is enough for every reconnect.
var mei0 = []byte{
	0x4d, 0x45, 0x49, 0x30, 0x01, 0x00, 0x10, 0x00,
	0x80, 0x01, 0x02, 0x00, 0x00, 0x03, 0x0b, 0x54,
}

// namePrefixes are how a Lumix camera names itself while it advertises.
var namePrefixes = []string{"S5-", "G100", "G110", "GH", "G9", "S1", "S9", "DC-"}

// IsLumix reports whether a device is a Lumix camera: it advertises the
// Location service, or names itself like one.
func IsLumix(d Device) bool {
	for _, uuid := range d.UUIDs {
		if strings.EqualFold(uuid, ServiceLocation) {
			return true
		}
	}
	for _, prefix := range namePrefixes {
		if strings.HasPrefix(d.Name, prefix) {
			return true
		}
	}
	return false
}

// Wake connects to the camera and runs the handshake that the app performs,
// which is what makes the camera bring its Wi-Fi up. It holds the connection
// while hold lasts, so the camera has time to start its access point, and
// then disconnects.
func Wake(ctx context.Context, d Device, hold time.Duration) error {
	central, err := NewCentral()
	if err != nil {
		return err
	}
	defer central.Close()

	conn, err := central.Connect(ctx, d)
	if err != nil {
		return err
	}
	defer conn.Disconnect()

	if err := conn.Handshake(ctx); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(hold):
	}
	return nil
}

// Handshake runs the app's init sequence: subscribe to the camera's ready
// notifications, send MEI0 and this client's name, wait for both ready
// notifications, and set the camera's clock. Shortening it makes the camera
// drop the connection, so the order matters.
//
// The subscriptions are best effort: some firmware refuses them until the
// session write has gone through, so a failure there does not end the
// handshake.
func (c *Conn) Handshake(ctx context.Context) error {
	ready := make(chan string, 4)
	if stop, err := c.Subscribe(uuidGPSReady, func([]byte) { ready <- "location" }); err == nil {
		defer stop()
	}
	if stop, err := c.Subscribe(uuidClkReady, func([]byte) { ready <- "clock" }); err == nil {
		defer stop()
	}

	if err := c.Write(uuidSession, mei0); err != nil {
		return fmt.Errorf("start the session: %w", err)
	}
	if err := c.Write(uuidDevName, []byte("kupic")); err != nil {
		return fmt.Errorf("name this client: %w", err)
	}

	// The camera announces readiness on both services before it takes the
	// clock; writing earlier is what makes it hang up.
	if err := c.awaitReady(ctx, ready); err != nil {
		return err
	}
	if err := c.Write(uuidClock, clockPacket(time.Now())); err != nil {
		return fmt.Errorf("set the camera's clock: %w", err)
	}
	return nil
}

// awaitReady waits for the camera to say both services are ready.
func (c *Conn) awaitReady(ctx context.Context, ready <-chan string) error {
	seen := map[string]bool{}
	timeout := time.After(8 * time.Second)
	for len(seen) < 2 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			// Some firmware may not notify; the session is still usable.
			return nil
		case which := <-ready:
			seen[which] = true
		}
	}
	return nil
}

// clockPacket builds the 10-byte packet that sets the camera's clock, in UTC.
func clockPacket(t time.Time) []byte {
	t = t.UTC()
	return []byte{
		byte(t.Year()), byte(t.Year() >> 8),
		byte(t.Month()), byte(t.Day()),
		byte(t.Hour()), byte(t.Minute()), byte(t.Second()),
		0x3c, 0x00, 0x01,
	}
}
