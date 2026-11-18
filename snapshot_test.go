package main

import (
	"context"
	"fmt"
	"hash/fnv"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/KhalidFinny/kupic/internal/ble"
	"github.com/KhalidFinny/kupic/internal/camera"
	"github.com/KhalidFinny/kupic/internal/card"
	"github.com/KhalidFinny/kupic/internal/convert"
)

// Snapshots render the app's pages to PNG files, for looking at them. They
// run only when KUPIC_SNAPSHOTS names a directory:
//
//	KUPIC_SNAPSHOTS=/tmp/kupic-shots go test -run TestSnapshots .
func TestSnapshots(t *testing.T) {
	dir := os.Getenv("KUPIC_SNAPSHOTS")
	if dir == "" {
		t.Skip("set KUPIC_SNAPSHOTS to a directory to write snapshots")
	}

	a, tt := shotApp()
	save(t, tt, dir, "pair-start")

	a, tt = shotApp()
	a.devices = sampleDevices()
	save(t, tt, dir, "pair-found")

	a, tt = shotApp()
	a.camera = "G100D-3D5833"
	a.devices = sampleDevices()
	save(t, tt, dir, "start")

	a, tt = shotApp()
	a.camera = "G100D-3D5833"
	a.phase = phaseScanning
	save(t, tt, dir, "scanning")

	a, tt = shotApp()
	a.camera = "G100D-3D5833"
	a.phase = phaseWaking
	save(t, tt, dir, "waking")

	a, tt = shotApp()
	a.camera = "G100D-3D5833"
	a.phase = phaseJoining
	save(t, tt, dir, "joining")

	a, tt = shotApp()
	a.camera = "G100D-3D5833"
	a.phase = phaseStart
	a.err = "No Lumix answered. Turn the camera's Wi-Fi on, choose [Send Images To PC], and try again."
	save(t, tt, dir, "error")

	a, tt = shotApp()
	a.camera = "G100D-3D5833"
	a.phase = phaseReady
	a.src = nil
	save(t, tt, dir, "grid")

	a, tt = shotApp()
	a.camera = "G100D-3D5833"
	a.phase = phaseReady
	for _, p := range a.photos[:3] {
		a.chosen.Add(p.ID)
	}
	save(t, tt, dir, "grid-selected")

	a, tt = shotApp()
	a.camera = "G100D-3D5833"
	a.phase = phaseReady
	a.selecting = true
	for _, p := range a.photos[:3] {
		a.chosen.Add(p.ID)
	}
	save(t, tt, dir, "grid-selecting")

	// The camera's own network, joined: there is no internet on it, so the
	// way back sits beside the read.
	a, tt = shotApp()
	a.camera = "G100D-3D5833"
	a.joined, a.home = "G100D-3D5833", "HomeNet"
	a.phase = phaseReady
	save(t, tt, dir, "grid-joined")

	a, tt = shotApp()
	a.camera = "G100D-3D5833"
	a.phase = phaseReady
	for _, p := range a.photos[:3] {
		a.chosen.Add(p.ID)
	}
	a.showImport = true
	a.importing = true
	a.done, a.total, a.progress = 4, 12, 4.0/12.0
	save(t, tt, dir, "import-progress")

	a, tt = shotApp()
	a.camera = "G100D-3D5833"
	a.phase = phaseReady
	a.showImport = true
	a.result = "Saved 12 photos to /home/finny/Pictures/kupic"
	save(t, tt, dir, "import-done")

	a, tt = shotApp()
	a.camera = "G100D-3D5833"
	a.phase = phaseReady
	a.photos = nil
	save(t, tt, dir, "empty")

	a, tt = usbApp(t)
	save(t, tt, dir, "usb-grid")

	// The same screens in a phone's window: the compact shell puts the
	// actions along the bottom and the grid three across.
	a, tt = phoneApp()
	save(t, tt, dir, "phone-start")

	a, tt = phoneApp()
	a.camera = "G100D-3D5833"
	a.phase = phaseReady
	save(t, tt, dir, "phone-grid")

	a, tt = phoneApp()
	a.camera = "G100D-3D5833"
	a.phase = phaseReady
	a.selecting = true
	for _, p := range a.photos[:3] {
		a.chosen.Add(p.ID)
	}
	save(t, tt, dir, "phone-selected")

	a, tt = phoneApp()
	a.devices = sampleDevices()
	save(t, tt, dir, "phone-pairing")

	a, tt = phoneApp()
	a.camera = "G100D-3D5833"
	a.phase = phaseReady
	a.selecting = true
	for _, p := range a.photos[:3] {
		a.chosen.Add(p.ID)
	}
	a.showImport = true
	save(t, tt, dir, "phone-import")
}

// usbApp is an app that has read a card over USB: the photos come from the real
// card package, over a card written to a temporary directory, so the snapshot
// exercises the path a plugged-in camera takes.
func usbApp(t *testing.T) (*app, *ui.Tester) {
	t.Helper()
	a, tt := shotApp()
	a.camera, a.joined, a.wifiPassword = "", "", ""
	a.photos = nil
	a.thumbs = newThumbCache()

	root := t.TempDir()
	folder := filepath.Join(root, "DCIM", "100_PANA")
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		name := fmt.Sprintf("P100%04d", 4200-i)
		writeJPEG(t, filepath.Join(folder, name+".JPG"), name)
		if i%2 == 0 {
			// A shot taken as RAW+JPEG, so the photo has two files.
			writeJPEG(t, filepath.Join(folder, name+".RW2"), name+"-raw")
		}
	}

	c, err := card.At(root)
	if err != nil {
		t.Fatal(err)
	}
	a.src = c
	if a.photos, err = c.List(context.Background()); err != nil {
		t.Fatal(err)
	}
	// The grid asks for thumbnails as it draws; a snapshot is one frame, so
	// they are loaded first.
	for _, photo := range a.photos {
		a.loadThumb(context.Background(), photo)
	}
	a.phase = phaseReady
	return a, tt
}

// writeJPEG writes a small real JPEG, so the card path has something to read
// back and decode.
func writeJPEG(t *testing.T, path, seed string) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := jpeg.Encode(file, sampleImage(seed), nil); err != nil {
		t.Fatal(err)
	}
}

// sampleDevices is what a Bluetooth scan of the room finds.
func sampleDevices() []ble.Device {
	return []ble.Device{
		{Name: "G100D-3D5833", Address: "6A:C3:46:63:7A:53", RSSI: -38},
		{Name: "BLUETOOTH_AUDIO", Address: "FC:84:A7:3D:10:BB", RSSI: -52},
	}
}

// shotApp is an app with a card's worth of photos, ready to draw.
func shotApp() (*app, *ui.Tester) { return shotAppSized(1180, 760) }

// phoneApp is the same app drawn in a phone's window, for the compact shell.
func phoneApp() (*app, *ui.Tester) { return shotAppSized(390, 844) }

// shotAppSized is an app drawn in a window of the given size.
func shotAppSized(width, height int) (*app, *ui.Tester) {
	a := newApp()
	// newApp reads the user's settings, which would make a snapshot depend
	// on what the machine last paired: start each one unpaired instead.
	a.camera, a.joined, a.wifiPassword = "", "", ""
	a.devices = nil
	a.dir = "/home/finny/Pictures/kupic"
	a.format = convert.JPEG
	a.quality = 90
	a.photos = samplePhotos(23)
	for _, p := range a.photos {
		a.setThumb(p.ID, sampleThumb(p.ID))
	}
	tt := ui.NewTester(a.view, width, height)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true, TextScale: 1})
	tt.SetDark(true)
	return a, tt
}

// samplePhotos is a card of photos, newest first, half of them shot as RAW.
func samplePhotos(n int) []camera.Photo {
	photos := make([]camera.Photo, 0, n)
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("P100%04d", 4200-i)
		p := camera.Photo{
			ID:    fmt.Sprintf("card/%s", name),
			Title: name,
			Files: []camera.File{{
				Name: name + ".JPG", MIME: "image/jpeg", Size: 6 << 20,
				URL: "http://192.168.54.1:60606/card/" + name + ".JPG",
			}},
		}
		if i%2 == 0 {
			p.Files = append(p.Files, camera.File{
				Name: name + ".RW2", MIME: "image/x-panasonic-rw2", Size: 26 << 20,
				URL: "http://192.168.54.1:60606/card/" + name + ".RW2",
			})
		}
		photos = append(photos, p)
	}
	return photos
}

// sampleThumb draws a stand-in for a photograph.
func sampleThumb(seed string) *ui.Bitmap { return ui.NewBitmap(sampleImage(seed)) }

// sampleImage draws a stand-in for a photograph: a two-colour wash with a
// disc in it, different for every name.
func sampleImage(seed string) image.Image {
	h := fnv.New32a()
	h.Write([]byte(seed))
	n := h.Sum32()

	const size = 400
	hue := float64(n % 360)
	top := hsl(hue, 0.34, 0.36)
	bottom := hsl(math.Mod(hue+45, 360), 0.40, 0.13)
	cx := float64(70 + n%260)
	cy := float64(70 + (n/7)%260)

	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			c := mixRGB(top, bottom, float64(x+y)/(2*size))
			if d := math.Hypot(float64(x)-cx, float64(y)-cy); d < 92 {
				c = mixRGB(c, color.RGBA{R: 255, G: 255, B: 255, A: 255}, 0.16)
			}
			img.Set(x, y, c)
		}
	}
	return img
}

// hsl is a colour of a hue in degrees, a saturation and a lightness, both
// from 0 to 1.
func hsl(hue, saturation, lightness float64) color.RGBA {
	c := (1 - math.Abs(2*lightness-1)) * saturation
	x := c * (1 - math.Abs(math.Mod(hue/60, 2)-1))
	m := lightness - c/2
	var r, g, b float64
	switch {
	case hue < 60:
		r, g, b = c, x, 0
	case hue < 120:
		r, g, b = x, c, 0
	case hue < 180:
		r, g, b = 0, c, x
	case hue < 240:
		r, g, b = 0, x, c
	case hue < 300:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}
	return color.RGBA{R: byte((r + m) * 255), G: byte((g + m) * 255), B: byte((b + m) * 255), A: 255}
}

// mixRGB blends two colours.
func mixRGB(a, b color.RGBA, t float64) color.RGBA {
	lerp := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*t) }
	return color.RGBA{R: lerp(a.R, b.R), G: lerp(a.G, b.G), B: lerp(a.B, b.B), A: 255}
}

// save draws a frame and writes it.
func save(t *testing.T, tt *ui.Tester, dir, name string) {
	t.Helper()
	tt.Frame()
	file, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := png.Encode(file, tt.Image()); err != nil {
		t.Fatal(err)
	}
}
