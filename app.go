package main

import (
	"context"
	"io"
	"os"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/KhalidFinny/kupic/internal/ble"
	"github.com/KhalidFinny/kupic/internal/camera"
	"github.com/KhalidFinny/kupic/internal/convert"
)

// source is where the photos come from: a camera over Wi-Fi, or its card over
// USB. Both list photos and open one for reading, so the grid and the import
// do not care which it is.
type source interface {
	Name() string
	List(ctx context.Context) ([]camera.Photo, error)
	Open(ctx context.Context, fileURL string) (io.ReadCloser, error)
}

// phase is what the app is doing.
type phase int

const (
	phaseStart       phase = iota // waiting for the user to look for a camera
	phasePairing                  // looking for cameras over Bluetooth
	phaseRegistering              // connecting to and registering one
	phaseScanning                 // looking for the camera on the network
	phaseWaking                   // waking the camera over Bluetooth
	phaseJoining                  // joining the camera's own network
	phaseConnecting
	phaseReading
	phaseReady
)

// app is the state the window shows.
type app struct {
	win   *mygo.Window
	theme *ui.Theme
	// compact is a phone, or a window too small for the desktop layout:
	// the shell puts the actions along the bottom, where a thumb reaches.
	compact bool

	// The source: the camera session or the card, what is on it, and what
	// went wrong reaching it.
	src   source
	phase phase
	err   string
	// devices are the cameras the last Bluetooth scan found, and camera
	// is the one registered, by name: the camera rotates its Bluetooth
	// address, so its name is what lasts.
	devices []ble.Device
	camera  string
	// joined is the camera's own network this machine joined, empty when
	// it is on its usual network.
	joined       string
	wifiPassword string
	// home is the network the machine was on before joining the camera's
	// own access point, which has no uplink: it is what puts the internet
	// back when the work is done.
	home string

	photos []camera.Photo
	// thumbs are the ones decoded, and the photos known to have none, kept
	// to a few screens' worth: a card of thousands would otherwise keep
	// every bitmap it ever drew.
	thumbs thumbCache
	// want is the thumbnails the grid is waiting for, which a worker takes:
	// only what the grid draws is fetched.
	want chan camera.Photo

	grid     ui.GridState
	selected int
	chosen   ui.Selection[string]
	// selecting is selection mode: a tap adds a photo or takes it out,
	// where a tap otherwise chooses one alone. A phone has no Cmd or
	// Shift, so this is the only way to choose several there.
	selecting bool

	// What an import writes: where, in which format, at what quality, and
	// whether the RAW file comes along.
	dir     string
	format  convert.Format
	quality int
	alsoRAW bool

	// The import in progress.
	importing  bool
	showImport bool
	progress   float64
	done       int
	total      int
	result     string

	// cancel ends the operation in flight, and with it the thumbnail
	// workers it started.
	cancel context.CancelFunc
	// run runs work off the main thread; tests replace it with a runner
	// that runs the work inline.
	run func(work func())
}

func newApp() *app {
	a := &app{
		theme:    leicaTheme(),
		thumbs:   newThumbCache(),
		want:     make(chan camera.Photo, 512),
		selected: -1,
		format:   convert.JPEG,
		quality:  90,
		dir:      picturesDir(),
	}
	a.loadSettings()
	// The folder an import writes to exists before it is needed, so the
	// dialog shows a real place rather than a path that has to be made. A
	// failure here surfaces at the import, which reports it.
	os.MkdirAll(a.dir, 0o755)
	a.grid.Key = func(i int) any { return a.photos[i].ID }
	a.grid.Selection = &a.chosen
	a.grid.Label = func(i int) string { return a.photos[i].Title }
	a.grid.Selected = &a.selected
	a.run = func(work func()) { go work() }
	return a
}

// busy reports whether the app is waiting on the camera.
func (a *app) busy() bool {
	switch a.phase {
	case phasePairing, phaseRegistering, phaseScanning, phaseWaking, phaseJoining, phaseConnecting, phaseReading:
		return true
	}
	return false
}

// registered reports whether a camera has been paired.
func (a *app) registered() bool { return a.camera != "" }

// update runs fn where the view can see its result, on the main thread, and
// draws a frame. Without a window, as in tests, it runs fn at once.
func (a *app) update(fn func()) {
	if a.win == nil {
		fn()
		return
	}
	a.win.Update(fn)
}

// work starts the context for one operation, ending the one before it. A
// thumbnail worker outliving the card it was started for would fetch photos
// that are no longer shown, from a source that is gone.
func (a *app) work() context.Context {
	a.stop()
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	return ctx
}

// stop ends whatever the app is doing off the main thread.
func (a *app) stop() {
	if a.cancel != nil {
		a.cancel()
		a.cancel = nil
	}
}

// fail returns to the start, saying what went wrong.
func (a *app) fail(message string) {
	a.update(func() {
		a.phase = phaseStart
		a.err = message
	})
}

// connection is what the status chip says, and the color of its dot.
func (a *app) connection() (string, ui.Color) {
	switch a.phase {
	case phasePairing:
		return "Looking for cameras…", warn
	case phaseRegistering:
		return "Pairing…", warn
	case phaseScanning:
		return "Looking for a camera…", warn
	case phaseWaking:
		return "Waking the camera…", warn
	case phaseJoining:
		return "Joining the camera…", warn
	case phaseConnecting:
		return "Connecting…", warn
	case phaseReading:
		return "Reading the card…", warn
	case phaseReady:
		return a.headline() + " · connected", ok
	}
	if a.err != "" {
		return "Not connected", bad
	}
	if a.registered() {
		return a.camera + " · paired", ok
	}
	return "No camera", muted
}

// headline names what the toolbar is about.
func (a *app) headline() string {
	switch a.phase {
	case phasePairing, phaseRegistering:
		return "Pair your camera"
	case phaseScanning, phaseWaking, phaseJoining, phaseConnecting:
		return "Looking for your camera"
	case phaseReading:
		return "Reading the card"
	case phaseReady:
		if a.src != nil && a.src.Name() != "" {
			return a.src.Name()
		}
		if a.registered() {
			return a.camera
		}
		return "Your Lumix"
	}
	if a.registered() {
		return a.camera
	}
	return "kupic"
}

// subhead is the line under the headline.
func (a *app) subhead() string {
	switch {
	case a.phase == phasePairing:
		return "This takes a moment over Bluetooth"
	case a.phase == phaseRegistering:
		return "Connecting and registering"
	case a.busy():
		return "This takes a moment over Wi-Fi"
	case a.phase == phaseReady && len(a.photos) > 0:
		if a.joined != "" {
			// The camera's access point has no uplink: while the machine
			// is on it, there is no internet. Say so where it is read.
			return plural(len(a.photos), "photo") + " on the card · on the camera's own network, no internet"
		}
		return plural(len(a.photos), "photo") + " on the card"
	case a.phase == phaseReady:
		return "The card is empty"
	}
	return "Photos straight off the camera, over Wi-Fi"
}

// busyMessage says what the app is waiting for.
func (a *app) busyMessage() string {
	switch a.phase {
	case phasePairing:
		return "Looking for your camera over Bluetooth…"
	case phaseRegistering:
		return "Connecting to the camera…"
	case phaseScanning:
		return "Looking for your camera on this network…"
	case phaseWaking:
		return "Waking the camera over Bluetooth…"
	case phaseJoining:
		return "Joining the camera's own network…"
	case phaseConnecting:
		return "Pairing with the camera…"
	case phaseReading:
		return "Reading the card…"
	}
	return "Working…"
}

// selectedPhotos returns the chosen photos, in the order they are shown.
func (a *app) selectedPhotos() []camera.Photo {
	var out []camera.Photo
	for _, p := range a.photos {
		if a.chosen.Has(p.ID) {
			out = append(out, p)
		}
	}
	return out
}
