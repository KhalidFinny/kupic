package main

import "github.com/egoist/mygo/ui"

// body is what a window shows between its toolbar and its actions. Both
// shells show it: it is the app's state, not its chrome.
func (a *app) body(c *ui.Context) {
	ui.Box(c).Grow(1).MinHeight(0).Children(func() {
		switch {
		case a.busy():
			busyBlock(c, a.busyMessage())
		case a.phase == phaseReady && len(a.photos) == 0:
			stateBlock(c, iconImage, muted, "The card is empty", "Shoot something, then rescan.", "Rescan", a.scan)
		case a.phase == phaseReady:
			// The card was read, so show it whatever the camera is
			// registered as: the Wi-Fi path never pairs one.
			a.photoGrid(c)
		case !a.registered():
			if a.compact {
				a.compactPairing(c)
			} else {
				a.pairing(c)
			}
		case a.err != "":
			stateBlock(c, iconCamera, bad, "No camera yet", a.err, "Try again", a.scan)
		default:
			stateBlock(c, iconCamera, muted, "Bring your camera over",
				"Turn the camera's Wi-Fi on, then press Find my camera. On a Lumix that is [Wi-Fi] → [Wi-Fi Function] → [New Connection] → [Remote Shooting & View]. With Bluetooth on, kupic can also wake the camera itself.",
				"Find my camera", a.scan)
		}
	})
}

// pairing offers the ways to reach a camera: its card over USB, which is the
// simplest and needs nothing set up on the camera, or the wireless paths. The
// password field belongs to the Wi-Fi one, so it stays on the screen either
// way.
func (a *app) pairing(c *ui.Context) {
	ui.Column(c).Fill().Gap(0).Padding(28, 22, 20, 22).Children(func() {
		ui.Text(c, "Bring your camera over").FontSize(17).Bold().TextColor(text)
		ui.Text(c, "Plug the camera in over USB, or reach it over Wi-Fi or Bluetooth.").
			FontSize(12).TextColor(muted)
		ui.Box(c).Height(16)
		if len(a.devices) == 0 {
			title, body := a.pairingCopy()
			stateBlock(c, iconUSB, muted, title, body, "Read over USB", a.readUSB)
		} else {
			ui.Scroll(c).Grow(1).Gap(8).Children(func() {
				for _, d := range a.devices {
					deviceRow(c, d, func() { a.pairCamera(d) })
				}
			})
		}
		wifiPasswordRow(c, &a.wifiPassword, a.saveSettings)
	})
}

// pairingCopy is what the pairing screen says: how to get the camera ready,
// or what went wrong last time.
func (a *app) pairingCopy() (title, body string) {
	title, body = "Plug in the camera",
		"Connect it with the USB cable and turn it on. If the camera asks, choose [PC(Storage)]: kupic reads the card straight off the drive it shows up as. Without a cable, use Connect over Wi-Fi or Find cameras."
	if a.err != "" {
		title, body = "No camera yet", a.err
	}
	return title, body
}
