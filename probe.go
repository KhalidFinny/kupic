package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/KhalidFinny/kupic/internal/ble"
	"github.com/KhalidFinny/kupic/internal/camera"
	"github.com/KhalidFinny/kupic/internal/wifi"
)

// probe is the app's Bluetooth check:
//
//	kupic probe
//
// It looks for a Lumix over Bluetooth, connects, prints the GATT the camera
// exposes, runs the handshake, and then looks again for the camera's Wi-Fi.
// It is how the Bluetooth path is confirmed on a real camera, since the
// desktop app cannot be driven without one.
func probe() {
	fmt.Println("kupic probe — looking for a Lumix over Bluetooth")

	central, err := ble.NewCentral()
	if err != nil {
		fmt.Println("Bluetooth is not available:", err)
		os.Exit(1)
	}
	defer central.Close()
	fmt.Println("adapter:", central.Adapter())

	ctx := context.Background()
	devices, err := central.Scan(ctx, 10*time.Second, ble.IsLumix)
	if err != nil {
		fmt.Println("scanning failed:", err)
	}
	if len(devices) == 0 {
		fmt.Println("\nNo Lumix advertised. On the camera, turn Bluetooth on and")
		fmt.Println("set it to pairing standby, then run this again.")
		os.Exit(2)
	}

	for _, d := range devices {
		fmt.Printf("\nfound %s  %s  rssi=%d\n", d.Name, d.Address, d.RSSI)
		for _, uuid := range d.UUIDs {
			fmt.Println("  advertises", uuid)
		}
	}

	d := devices[0]
	// The camera rotates its Bluetooth address (a resolvable private
	// address), so a name match is the reliable one; the address seen a
	// moment ago may already be gone.
	for _, candidate := range devices {
		if strings.HasPrefix(candidate.Name, "G100") || strings.Contains(candidate.Name, "LUMIX") {
			d = candidate
			break
		}
	}
	fmt.Printf("\nconnecting to %s (%s)…\n", d.Name, d.Address)
	conn, err := central.Connect(ctx, d)
	if err != nil {
		fmt.Println("connect failed:", err)
		fmt.Println("\nThe camera is advertising but will not accept a connection.")
		fmt.Println("On the camera: turn [Bluetooth] ON, and make sure it is not")
		fmt.Println("already connected to a phone running LUMIX Sync.")
		os.Exit(1)
	}
	defer conn.Disconnect()

	services, _ := conn.Services()
	chars, _ := conn.Characteristics()
	fmt.Printf("services: %d, characteristics: %d\n", len(services), len(chars))
	for _, uuid := range chars {
		flags, _ := conn.Flags(uuid)
		fmt.Printf("  char %s  [%s]\n", uuid, strings.Join(flags, ","))
	}

	fmt.Println("\nrunning the handshake…")
	if err := conn.Handshake(ctx); err != nil {
		fmt.Println("handshake failed:", err)
	} else {
		fmt.Println("handshake accepted; the camera did not hang up.")
	}
	fmt.Printf("still connected: %v\n", conn.Connected())
	if !conn.Connected() {
		fmt.Println("the camera dropped the connection — it rejected the handshake.")
		os.Exit(1)
	}

	// Hold the connection: the camera brings its Wi-Fi up while the app is
	// connected, so dropping it immediately can cancel that.
	fmt.Println("\nholding the connection while the camera starts its Wi-Fi…")
	select {
	case <-ctx.Done():
	case <-time.After(8 * time.Second):
	}
	conn.Disconnect()

	// The camera is in [Direct] mode by default: it started its own
	// network, so this machine has to join it before the camera answers.
	fmt.Println("\nlooking for the camera's own network…")
	wifi.Rescan()
	ssid, err := wifi.CameraAP(d.Name)
	if err != nil {
		fmt.Println("no camera network in range:", err)
		os.Exit(2)
	}
	fmt.Println("found", ssid, "— joining…")
	if err := wifi.Join(ssid, password()); err != nil {
		fmt.Println("join failed:", err)
		os.Exit(1)
	}
	fmt.Println("joined", ssid)

	fmt.Println("\nlooking for the camera on that network…")
	if waitForWiFi(ctx, 20*time.Second) {
		fmt.Println("the camera answered — the whole path works.")
	} else {
		fmt.Println("nothing answered at", wifi.Address, "yet.")
	}
}

// password is the camera's own network's password, from the environment for
// the probe, since the probe has no interface to ask.
func password() string { return os.Getenv("KUPIC_WIFI_PASSWORD") }

// waitForWiFi looks for the camera over the network, for up to wait.
func waitForWiFi(ctx context.Context, wait time.Duration) bool {
	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		devices, err := camera.Discover(ctx, 3*time.Second)
		if err == nil && len(devices) > 0 {
			return true
		}
	}
	return false
}
