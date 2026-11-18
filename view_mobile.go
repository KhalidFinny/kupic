package main

import (
	"github.com/egoist/mygo/ui"

	"github.com/KhalidFinny/kupic/internal/card"
)

// compactView is the phone's shell: a slim header for identity and status,
// the content at full width, and the actions along the bottom, where a thumb
// reaches. There is no title bar to drag and no toolbar to crowd.
func (a *app) compactView(c *ui.Context) {
	ui.Column(c).Fill().Children(func() {
		a.compactHeader(c)
		a.body(c)
		a.compactDock(c)
		importDialog(c, a.importDialog())
	})
}

// compactHeader names the app and says what the camera is doing. It is
// content, not window chrome: a phone has no title bar to drag.
func (a *app) compactHeader(c *ui.Context) {
	label, dot := a.connection()
	ui.Row(c).Height(52).Padding(0, 16, 0, 16).Gap(9).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, markAperture).FontSize(20).TextColor(text)
		ui.Text(c, "kupic").FontSize(15).Bold().TextColor(text)
		ui.Spacer(c)
		ui.Box(c).Size(7, 7).Radius(999).Background(dot)
		ui.Text(c, label).FontSize(12).TextColor(muted).SingleLine()
	})
}

// compactDock holds the phase's main action at the bottom of the screen,
// full width and tall enough for a finger.
func (a *app) compactDock(c *ui.Context) {
	if a.busy() {
		return
	}
	ui.Column(c).Gap(8).Padding(10, 16, 14, 16).Border(1, line).Children(func() {
		switch {
		case a.phase == phaseReady && a.selecting:
			a.compactSelectionDock(c)
		case a.phase == phaseReady:
			// Reading the card again means whatever got here: a mounted
			// card, or the camera on the network.
			ui.Row(c).Gap(8).Children(func() {
				label, run := "Rescan", a.scan
				if _, ok := a.src.(*card.Card); ok {
					label, run = "Read again", a.readUSB
				}
				ui.PrimaryButton(c, label).Grow(1).Height(touch).OnClick(run)
				if len(a.photos) > 0 {
					ui.Button(c, "Select").Height(touch).OnClick(func() { a.selecting = true })
				}
			})
			if a.joined != "" {
				ui.Button(c, "Back online").Height(touch).OnClick(a.leaveCamera)
			}
		case !a.registered():
			ui.PrimaryButton(c, "Read over USB").Height(touch).OnClick(a.readUSB)
		default:
			ui.PrimaryButton(c, "Find my camera").Height(touch).OnClick(a.scan)
		}
	})
}

// compactSelectionDock is what a phone shows in selection mode: the count and
// the way out above, then the two things to do with the choice, each wide
// enough for a thumb.
func (a *app) compactSelectionDock(c *ui.Context) {
	ui.Row(c).AlignItems(ui.Center).Children(func() {
		ui.Textf(c, "%d selected", a.chosen.Len()).FontSize(13).Bold().TextColor(text)
		ui.Spacer(c)
		ui.Button(c, "Done").Height(40).OnClick(func() { a.selecting = false })
	})
	ui.Row(c).Gap(8).Children(func() {
		ui.Button(c, "Clear").Grow(1).Height(touch).Disabled(a.chosen.Len() == 0).OnClick(a.chosen.Clear)
		ui.PrimaryButton(c, "Import").Grow(1).Height(touch).
			Disabled(a.chosen.Len() == 0).OnClick(a.openImport)
	})
}

// touch is the height a control needs for a fingertip, in DIPs.
const touch = 48

// compactPairing is the pairing screen laid out for a phone: everything
// stacked, and the two wireless ways in beside the cable rather than in a
// toolbar that does not exist here.
func (a *app) compactPairing(c *ui.Context) {
	title, body := a.pairingCopy()
	ui.Scroll(c).Fill().Children(func() {
		ui.Column(c).Gap(10).Padding(14, 16, 16, 16).Children(func() {
			ui.Text(c, title).FontSize(19).Bold().TextColor(text)
			ui.Text(c, body).FontSize(13).TextColor(muted)
			ui.Box(c).Height(6)
			if len(a.devices) == 0 {
				ui.Button(c, "Connect over Wi-Fi").Height(touch).OnClick(a.scan)
				ui.Button(c, "Find cameras").Height(touch).OnClick(a.findCameras)
			} else {
				ui.Text(c, "Cameras found").FontSize(12).TextColor(muted)
				for _, d := range a.devices {
					deviceCard(c, d, func() { a.pairCamera(d) })
				}
			}
			passwordField(c, &a.wifiPassword, a.saveSettings)
		})
	})
}
