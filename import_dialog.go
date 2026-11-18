package main

import (
	"github.com/egoist/mygo/ui"

	"github.com/KhalidFinny/kupic/internal/convert"
)

// importDialogState is what the import dialog shows, and what its controls do.
type importDialogState struct {
	// open is the dialog's shown flag, which a dismissal clears.
	open        *bool
	chosen      int
	format      convert.Format
	quality     int
	alsoRAW     *bool
	dir         string
	importing   bool
	progress    float64
	done, total int
	result      string

	onQuality   func(int)
	onSaved     func()
	onChooseDir func()
	onImport    func()
	onClose     func()
}

// importDialog is the dialog as the app has it now.
func (a *app) importDialog() importDialogState {
	return importDialogState{
		open:        &a.showImport,
		chosen:      a.chosen.Len(),
		format:      a.format,
		quality:     a.quality,
		alsoRAW:     &a.alsoRAW,
		dir:         a.dir,
		importing:   a.importing,
		progress:    a.progress,
		done:        a.done,
		total:       a.total,
		result:      a.result,
		onQuality:   a.setQuality,
		onSaved:     a.saveSettings,
		onChooseDir: a.chooseDir,
		onImport:    a.runImport,
		onClose:     a.closeImport,
	}
}

// importDialog asks where to write, in what, and how well, then reports how
// the import went.
func importDialog(c *ui.Context, s importDialogState) {
	ui.Modal(c, s.open, func() {
		ui.Column(c).Width(dialogWidth(c)).Gap(14).Children(func() {
			ui.Text(c, "Import photos").FontSize(19).Bold().TextColor(text)
			ui.Textf(c, "%d selected · %s", s.chosen, s.format.Label()).FontSize(12).TextColor(muted)

			switch {
			case s.importing:
				ui.Progress(c, s.progress).Label("Importing")
				ui.Textf(c, "%d of %d", s.done, s.total).FontSize(12).TextColor(muted)
			case s.result != "":
				ui.Text(c, s.result).FontSize(13).TextColor(text).MaxLines(3)
			}

			destinationRow(c, s.dir, s.onChooseDir)
			if s.format.Lossy() {
				if quality, ok := qualityRow(c, s.quality); ok {
					s.onQuality(quality)
				}
			}
			ui.Checkbox(c, s.alsoRAW, "Also copy the RAW file").OnChange(s.onSaved)

			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if s.importing {
					ui.Button(c, "Close").OnClick(s.onClose)
					return
				}
				ui.Button(c, "Cancel").OnClick(s.onClose)
				ui.PrimaryButton(c, "Import").Disabled(s.chosen == 0).OnClick(s.onImport)
			})
		})
	})
}

// dialogWidth keeps the import dialog inside a phone's screen. The modal's
// panel keeps 20 DIPs of padding either side inside a margin of 40, so the
// content has the window's width less 80 to work with.
func dialogWidth(c *ui.Context) float32 {
	width, _ := c.Size()
	return min(470, width-80)
}

// destinationRow shows the output folder and a way to change it.
func destinationRow(c *ui.Context, dir string, choose func()) {
	ui.Column(c).Gap(6).Children(func() {
		ui.Text(c, "Folder").FontSize(12).TextColor(muted)
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Grow(1).MinWidth(0).Padding(8, 10).Radius(6).Background(surface).Border(1, line).Children(func() {
				ui.Text(c, dir).FontSize(12).TextColor(text).SingleLine()
			})
			ui.Button(c, "Choose…").OnClick(choose)
		})
	})
}

// qualityRow sets how hard a lossy format compresses, and reports a new value.
func qualityRow(c *ui.Context, quality int) (int, bool) {
	value := float64(quality)
	changed := false
	ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "Quality").FontSize(12).TextColor(muted).Width(56)
		changed = ui.Slider(c, &value, 40, 100).Changed()
		ui.Textf(c, "%d", int(value)).FontSize(12).TextColor(muted).Width(30)
	})
	if !changed {
		return quality, false
	}
	return int(value), true
}
