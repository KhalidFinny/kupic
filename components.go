package main

import (
	"github.com/egoist/mygo/ui"

	"github.com/KhalidFinny/kupic/internal/ble"
)

// The pieces both shells build. Each one takes what it shows and what its
// controls do, and holds no state of its own: the app's actions change it.

// stateBlock is a centered glyph, a title, a line of body text and an action.
func stateBlock(c *ui.Context, glyph *ui.SVG, tone ui.Color, title, body, action string, run func()) {
	ui.Column(c).Fill().Center().Gap(16).Padding(48, 32).Children(func() {
		ui.Box(c).Size(78, 78).Radius(22).Background(raised).Center().Children(func() {
			ui.Icon(c, glyph).FontSize(34).TextColor(tone)
		})
		ui.Text(c, title).FontSize(19).Bold().TextColor(text).TextAlign(ui.Center)
		if body != "" {
			ui.Text(c, body).FontSize(13).TextColor(muted).MaxLines(4).MaxWidth(470).TextAlign(ui.Center)
		}
		if action != "" && run != nil {
			ui.PrimaryButton(c, action).OnClick(run)
		}
	})
}

// busyBlock says what the app is waiting for.
func busyBlock(c *ui.Context, message string) {
	ui.Column(c).Fill().Center().Gap(16).Children(func() {
		ui.Spinner(c)
		ui.Text(c, message).FontSize(14).TextColor(muted)
	})
}

// statusChip says whether a camera is connected, with a dot.
func statusChip(c *ui.Context, label string, dot ui.Color) {
	ui.Row(c).Gap(7).Padding(5, 10).Radius(999).AlignItems(ui.Center).Background(raised).Children(func() {
		ui.Box(c).Size(7, 7).Radius(999).Background(dot)
		ui.Text(c, label).FontSize(12).TextColor(muted).SingleLine()
	})
}

// deviceRow is one camera the Bluetooth scan found, in the desktop's list.
func deviceRow(c *ui.Context, d ble.Device, onPair func()) {
	row := ui.Row(c).Key(d.Address).Padding(12, 14).Gap(14).Radius(12).
		Background(raised).Border(1, line).AlignItems(ui.Center)
	if row.Hovered() {
		row.Background(hover)
	}
	row.Children(func() {
		ui.Box(c).Size(40, 40).Radius(10).Background(surface).Center().Children(func() {
			ui.Icon(c, iconCamera).FontSize(20).TextColor(text)
		})
		ui.Column(c).Gap(2).Grow(1).MinWidth(0).Children(func() {
			ui.Text(c, d.Name).FontSize(14).Bold().TextColor(text).SingleLine()
			ui.Textf(c, "%s · %d dBm", d.Address, d.RSSI).FontSize(12).TextColor(muted).SingleLine()
		})
		ui.PrimaryButton(c, "Pair").OnClick(onPair)
	})
}

// deviceCard is the same camera as a card a finger can take the whole of.
func deviceCard(c *ui.Context, d ble.Device, onPair func()) {
	ui.Row(c).Key(d.Address).Padding(12, 14).Gap(12).Radius(10).Background(raised).
		Border(1, line).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Size(38, 38).Radius(9).Background(surface).Center().Children(func() {
			ui.Icon(c, iconCamera).FontSize(19).TextColor(text)
		})
		ui.Column(c).Gap(2).Grow(1).MinWidth(0).Children(func() {
			ui.Text(c, d.Name).FontSize(14).Bold().TextColor(text).SingleLine()
			ui.Textf(c, "%d dBm", d.RSSI).FontSize(12).TextColor(muted).SingleLine()
		})
		ui.PrimaryButton(c, "Pair").Height(40).OnClick(onPair)
	})
}

// wifiPasswordRow takes the password of the camera's own network, beside its
// label. A current camera always asks for one, and shows it on its screen.
func wifiPasswordRow(c *ui.Context, password *string, onChange func()) {
	ui.Row(c).Gap(12).AlignItems(ui.Center).PaddingY(10).Children(func() {
		ui.Text(c, "Wi-Fi password").FontSize(12).TextColor(muted).Width(96)
		ui.TextInput(c.Key("wifi-password"), password).
			Placeholder("The password the camera shows for its network").
			Password().Width(300).OnChange(onChange)
	})
}

// passwordField takes the same password full width: the desktop's
// label-beside-field row leaves too little for the field on a phone.
func passwordField(c *ui.Context, password *string, onChange func()) {
	ui.Column(c).Gap(6).Children(func() {
		ui.Text(c, "Wi-Fi password").FontSize(12).TextColor(muted)
		ui.TextInput(c.Key("wifi-password"), password).
			Placeholder("The password the camera shows for its network").
			Password().OnChange(onChange)
	})
}
