package main

import (
	"context"
	"os"
	"path/filepath"

	"github.com/KhalidFinny/kupic/internal/camera"
	"github.com/KhalidFinny/kupic/internal/card"
)

// readUSB reads the card straight off the drive it appears as. A camera set to
// [PC(Storage)] shows its card as a USB mass storage volume, so there is no
// session to open, nothing to authenticate, and no Bluetooth or Wi-Fi needed.
func (a *app) readUSB() {
	if a.busy() {
		return
	}
	ctx := a.work()
	a.update(func() {
		a.phase = phaseReading
		a.err = ""
	})
	a.run(func() {
		c, err := card.Find()
		if err != nil {
			a.fail("No card found. Connect the camera with its USB cable, turn it on, and set [USB Mode] to [PC(Storage)].")
			return
		}
		if err := a.readSource(ctx, c); err != nil {
			a.fail("Could not read the card: " + err.Error())
		}
	})
}

// readSource lists a source's photos and shows them. A listing that fails
// outright is returned to the caller, which knows what the source is and what
// is worth recording about it.
func (a *app) readSource(ctx context.Context, src source) error {
	a.update(func() {
		a.src = src
		a.phase = phaseReading
	})

	photos, err := src.List(ctx)
	if err != nil && len(photos) == 0 {
		return err
	}
	a.update(func() {
		a.photos = photos
		a.phase = phaseReady
		a.chosen.Clear()
		a.selected = -1
	})
	// The queue still holds the photos the last card was scrolled to.
	a.forgetThumbs()
	a.startThumbs(ctx)
	return nil
}

// failWithDiagnostics reports a failure and, when the camera still answers,
// saves the raw replies beside it. The window cannot show a full answer, and
// this protocol is only readable from the camera's own words.
func (a *app) failWithDiagnostics(ctx context.Context, device camera.Device, message string) {
	if report, ok := camera.Diagnose(ctx, device); ok {
		if path, err := writeDiagnostics(report); err == nil {
			message += " · saved in " + path
		}
	}
	a.fail(message)
}

// writeDiagnostics saves a diagnostic report where the user can find it: the
// home directory, which is what their file manager opens on.
func writeDiagnostics(report string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	path := filepath.Join(home, "kupic-wifi.txt")
	if err := os.WriteFile(path, []byte(report), 0o644); err != nil {
		return "", err
	}
	return path, nil
}
