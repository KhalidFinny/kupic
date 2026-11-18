package main

import (
	"github.com/egoist/mygo/ui"

	"github.com/KhalidFinny/kupic/internal/card"
	"github.com/KhalidFinny/kupic/internal/convert"
)

// view builds the window. A phone, or a window too small for the desktop
// layout, gets the compact shell: the actions along the bottom, where a
// thumb reaches, and no title bar to drag.
func (a *app) view(c *ui.Context) {
	c.SetTheme(a.theme)
	c.Root().Background(bg)

	width, height := c.Size()
	a.compact = width < compactWidth || height < compactHeight

	// In selection mode the grid's own clicks would collapse the choice on
	// every tap, so the app takes them instead.
	if a.selecting {
		a.grid.Selected = nil
	} else {
		a.grid.Selected = &a.selected
	}

	if a.compact {
		a.compactView(c)
	} else {
		a.desktopView(c)
	}
}

// compactWidth and compactHeight are where the layout gives up its toolbar
// and its title bar: a phone held either way, or a narrow window.
const (
	compactWidth  = 720
	compactHeight = 520
)

// desktopView is the window: a title bar the app draws itself, the camera's
// toolbar, the photos, and the import dialog over them.
func (a *app) desktopView(c *ui.Context) {
	bar := c.TitleBar()
	ui.Column(c).Fill().Children(func() {
		a.titleBar(c, bar)
		a.toolbar(c)
		a.body(c)
		a.actionBar(c)
		importDialog(c, a.importDialog())
	})
}

// titleBar is the strip under the window controls: the wordmark and what
// the camera is doing.
func (a *app) titleBar(c *ui.Context, bar ui.TitleBar) {
	ui.Row(c).Height(max(bar.Height, 44)).Padding(0, bar.Right+18, 0, bar.Left+18).
		Gap(9).AlignItems(ui.Center).DragWindow().Children(func() {
		ui.Icon(c, markAperture).FontSize(18).TextColor(text)
		ui.Text(c, "kupic").FontSize(14).Bold().TextColor(text)
		ui.Spacer(c)
		label, dot := a.connection()
		statusChip(c, label, dot)
	})
}

// toolbar names the camera and offers what to do with it.
func (a *app) toolbar(c *ui.Context) {
	ui.Row(c).PaddingX(22).Height(64).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Gap(3).Children(func() {
			ui.Text(c, a.headline()).FontSize(17).Bold().TextColor(text).SingleLine()
			ui.Text(c, a.subhead()).FontSize(12).TextColor(muted).SingleLine()
		})
		ui.Spacer(c)
		a.toolbarActions(c)
	})
}

// toolbarActions offers the ways in: the camera's card over USB, which needs
// no Bluetooth or Wi-Fi at all, then the two wireless ones.
func (a *app) toolbarActions(c *ui.Context) {
	if a.busy() {
		return
	}
	if a.phase == phaseReady {
		// Reading the card again means whatever got here: a mounted card,
		// or the camera on the network.
		if _, ok := a.src.(*card.Card); ok {
			ui.Button(c, "Read again").OnClick(a.readUSB)
		} else {
			ui.Button(c, "Rescan").OnClick(a.scan)
		}
		// On the camera's own network there is no internet, so the way
		// back belongs here, beside the read.
		if a.joined != "" {
			ui.Button(c, "Back online").OnClick(a.leaveCamera)
		}
		if a.selecting {
			ui.PrimaryButton(c, "Done").OnClick(func() { a.selecting = false })
		} else if len(a.photos) > 0 {
			ui.Button(c, "Select").OnClick(func() { a.selecting = true })
		}
		return
	}
	if !a.registered() {
		ui.PrimaryButton(c, "Read over USB").OnClick(a.readUSB)
		ui.Button(c, "Connect over Wi-Fi").OnClick(a.scan)
		ui.Button(c, "Find cameras").OnClick(a.findCameras)
		return
	}
	ui.PrimaryButton(c, "Find my camera").OnClick(a.scan)
}

// actionBar floats over the grid's foot while photos are chosen.
func (a *app) actionBar(c *ui.Context) {
	if a.phase != phaseReady || a.chosen.Len() == 0 {
		return
	}
	ui.Box(c).Padding(0, 22, 22, 22).Children(func() {
		ui.Row(c).Padding(12, 16).Gap(16).Radius(14).Background(raised).Border(1, line).
			AlignItems(ui.Center).Children(func() {
			ui.Textf(c, "%d selected", a.chosen.Len()).FontSize(13).Bold().TextColor(text)
			ui.Spacer(c)
			if format, ok := formatPicker(c, a.format); ok {
				a.setFormat(format)
			}
			ui.Button(c, "Clear").OnClick(a.chosen.Clear)
			ui.PrimaryButton(c, "Import").OnClick(a.openImport)
		})
	})
}

// formatLabels is what the picker's segments say, built once: the view runs
// every frame.
var formatLabels = func() []string {
	labels := make([]string, len(convert.Formats))
	for i, format := range convert.Formats {
		labels[i] = format.Label()
	}
	return labels
}()

// formatPicker chooses what an import writes, and reports a new choice.
func formatPicker(c *ui.Context, current convert.Format) (convert.Format, bool) {
	index := 0
	for i, format := range convert.Formats {
		if format == current {
			index = i
		}
	}
	chosen := index
	if !ui.Segmented(c, &chosen, formatLabels...).Changed() {
		return current, false
	}
	return convert.Formats[chosen], true
}
