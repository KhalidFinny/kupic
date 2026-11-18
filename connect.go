package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/KhalidFinny/kupic/internal/camera"
	"github.com/KhalidFinny/kupic/internal/wifi"
)

// readCard opens a session with a camera found on the network and reads its
// card.
func (a *app) readCard(ctx context.Context, device camera.Device) {
	client, err := camera.Connect(ctx, device, clientName)
	if err != nil {
		// The camera is there but will not let us in. Record what it
		// actually said: the access reply format is the one part of this
		// protocol that differs between firmware, and the window cannot
		// show it in full.
		a.failWithDiagnostics(ctx, device, "Could not connect to the camera: "+err.Error())
		return
	}
	if err := a.readSource(ctx, client); err != nil {
		// A listing that fails says almost nothing on its own: the camera's
		// own answer to the request is what identifies the fault.
		a.failWithDiagnostics(ctx, device, "Could not read the card: "+err.Error())
	}
}

// discoverAndRead finds the camera once Bluetooth has woken it, and reads
// its card. A camera in [Direct] mode is not on this machine's network: it
// started its own access point, so the machine joins it first, which is what
// LUMIX Sync does.
func (a *app) discoverAndRead(ctx context.Context) {
	a.update(func() { a.phase = phaseJoining })
	if err := a.joinCamera(ctx); err != nil {
		a.fail(err.Error())
		return
	}

	a.update(func() { a.phase = phaseScanning })
	var devices []camera.Device
	for attempt := 0; attempt < 8; attempt++ {
		devices, _ = camera.Discover(ctx, 3*time.Second)
		if len(devices) > 0 {
			break
		}
		if ctx.Err() != nil {
			return
		}
	}
	if len(devices) == 0 {
		a.fail("The camera's own network was joined, but nothing answered on it. Set the camera's Wi-Fi to [Direct] so it starts its own network, or to [Via Network] so it joins this one.")
		return
	}
	a.update(func() { a.phase = phaseConnecting })
	a.readCard(ctx, devices[0])
}

// joinCamera connects this machine to the camera's own access point, if it
// is not already on a network with the camera. The camera answers at a fixed
// address there, so a machine already on that network is left alone.
func (a *app) joinCamera(ctx context.Context) error {
	if ssid := a.camera; ssid != "" && wifi.ConnectedTo(ssid) {
		return nil
	}
	wifi.Rescan()
	ssid, err := wifi.CameraAP(a.camera)
	if err != nil {
		return fmt.Errorf("the camera's own network is not in range yet. On the camera, open [Wi-Fi] → [Wi-Fi Function] → [New Connection] → [Remote Shooting & View] so it shows its SSID, then try again")
	}
	// Remember what the machine was on: the camera's access point has no
	// uplink, so this is what puts the internet back afterwards.
	home, _ := wifi.Active()
	if err := wifi.Join(ssid, a.wifiPassword); err != nil {
		return err
	}
	a.update(func() {
		a.joined = ssid
		if home != "" && !strings.EqualFold(home, ssid) {
			a.home = home
		}
		// The camera's access point is named after the camera, so joining
		// it is enough to know which camera this is.
		if a.camera == "" {
			a.camera = ssid
			a.saveSettings()
		}
	})
	// NetworkManager finishes the association a moment after the command
	// returns; give the address a chance to arrive.
	select {
	case <-ctx.Done():
	case <-time.After(3 * time.Second):
	}
	return nil
}

// leaveCamera puts this machine back on the network it was on before the
// camera's own access point. That access point has no uplink, so while the
// machine is on it there is no internet at all; this is the way back.
func (a *app) leaveCamera() {
	if a.joined == "" || a.busy() {
		return
	}
	home := a.home
	a.run(func() {
		if err := wifi.Return(home); err != nil {
			a.update(func() { a.err = err.Error() })
			return
		}
		a.update(func() {
			a.joined, a.home = "", ""
			a.err = ""
			a.settleThumbs()
		})
	})
}
