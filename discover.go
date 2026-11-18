package main

import (
	"context"
	"time"

	"github.com/KhalidFinny/kupic/internal/ble"
	"github.com/KhalidFinny/kupic/internal/camera"
)

// clientName is what the camera shows on its own screen for this app.
const clientName = "kupic"

// findCameras looks for Lumix cameras over Bluetooth and lists them, so
// that the user can pick one to register. The camera rotates its Bluetooth
// address, so what is shown is its name.
func (a *app) findCameras() {
	if a.busy() {
		return
	}
	ctx := a.work()
	a.update(func() {
		a.phase = phasePairing
		a.err = ""
		a.devices = nil
	})
	a.run(func() {
		central, err := ble.NewCentral()
		if err != nil {
			a.fail("Bluetooth is not available: " + err.Error())
			return
		}
		defer central.Close()

		devices, err := central.Scan(ctx, 8*time.Second, ble.IsLumix)
		if err != nil {
			a.fail("Could not look for the camera: " + err.Error())
			return
		}
		a.update(func() {
			a.devices = devices
			a.phase = phaseStart
		})
	})
}

// pairCamera registers the chosen camera: it connects over Bluetooth, runs
// the handshake the camera expects, and remembers the camera by name. Then
// it goes on to read the card.
func (a *app) pairCamera(d ble.Device) {
	if a.busy() {
		return
	}
	ctx := a.work()
	a.update(func() {
		a.phase = phaseRegistering
		a.err = ""
	})
	a.run(func() {
		central, err := ble.NewCentral()
		if err != nil {
			a.fail("Bluetooth is not available: " + err.Error())
			return
		}
		defer central.Close()

		conn, err := central.Connect(ctx, d)
		if err != nil {
			a.fail("Could not connect to " + d.Name + ": " + err.Error())
			return
		}
		defer conn.Disconnect()

		// The handshake is what registers this client on the camera and
		// makes it bring its Wi-Fi up. A camera that refuses it is still
		// worth remembering: the Wi-Fi path may work on its own.
		handshakeErr := conn.Handshake(ctx)
		a.update(func() {
			a.camera = d.Name
			a.saveSettings()
		})

		// Hold the connection while the camera starts its Wi-Fi.
		select {
		case <-ctx.Done():
			return
		case <-time.After(8 * time.Second):
		}

		if handshakeErr != nil && !conn.Connected() {
			a.fail("The camera rejected the pairing: " + handshakeErr.Error())
			return
		}
		a.discoverAndRead(ctx)
	})
}

// scan looks for the camera on the network, connects to it, and reads its
// card. It runs off the main thread and draws as it goes.
func (a *app) scan() {
	if a.busy() {
		return
	}
	ctx := a.work()
	a.update(func() {
		a.phase = phaseScanning
		a.err = ""
	})
	a.run(func() {
		devices, err := camera.Discover(ctx, 3*time.Second)
		if err != nil {
			a.fail("Could not look for the camera: " + err.Error())
			return
		}
		if len(devices) > 0 {
			a.update(func() { a.phase = phaseConnecting })
			a.readCard(ctx, devices[0])
			return
		}

		// Nothing on this network. A camera already registered can be woken
		// over Bluetooth; one that is not has to be put into Wi-Fi standby
		// by hand, and this machine joins the network it starts.
		if a.registered() {
			a.update(func() { a.phase = phaseWaking })
			a.wakeCamera(ctx)
		}
		a.discoverAndRead(ctx)
	})
}

// wakeCamera asks the registered camera to come to life over Bluetooth, so
// that it brings its Wi-Fi up. It is best effort: without a Bluetooth
// adapter, or with no camera advertising, the Wi-Fi path is all that is
// left.
func (a *app) wakeCamera(ctx context.Context) {
	central, err := ble.NewCentral()
	if err != nil {
		return
	}
	defer central.Close()

	devices, err := central.Scan(ctx, 8*time.Second, ble.IsLumix)
	if err != nil || len(devices) == 0 {
		return
	}
	// The address rotates, so prefer the registered camera by name.
	d := devices[0]
	for _, candidate := range devices {
		if candidate.Name == a.camera {
			d = candidate
			break
		}
	}
	conn, err := central.Connect(ctx, d)
	if err != nil {
		return
	}
	defer conn.Disconnect()
	if err := conn.Handshake(ctx); err != nil {
		return
	}
	// Hold the connection while the camera starts its Wi-Fi: the app is
	// connected when the camera brings it up.
	select {
	case <-ctx.Done():
	case <-time.After(10 * time.Second):
	}
}
