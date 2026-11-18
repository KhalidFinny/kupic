package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"

	"github.com/KhalidFinny/kupic/internal/camera"
	"github.com/KhalidFinny/kupic/internal/convert"
)

// openImport opens the import dialog for the chosen photos.
func (a *app) openImport() {
	if a.chosen.Len() == 0 {
		return
	}
	a.progress, a.done, a.total, a.result = 0, 0, 0, ""
	a.showImport = true
}

// closeImport dismisses the import dialog, leaving an import that is still
// running to finish.
func (a *app) closeImport() { a.showImport = false }

// setFormat chooses what an import writes.
func (a *app) setFormat(format convert.Format) {
	a.format = format
	a.saveSettings()
}

// setQuality sets how hard a lossy format compresses.
func (a *app) setQuality(quality int) {
	a.quality = quality
	a.saveSettings()
}

// runImport downloads the chosen photos, converts them, and writes them to
// the output folder. It runs off the main thread and reports progress.
func (a *app) runImport() {
	if a.importing || a.src == nil {
		return
	}
	if a.format != convert.JPEG {
		if err := convert.Available(); err != nil {
			a.update(func() { a.result = "Converting needs ffmpeg, which is not installed." })
			return
		}
	}
	photos := a.selectedPhotos()
	if len(photos) == 0 {
		return
	}
	dir, format, quality, alsoRAW := a.dir, a.format, a.quality, a.alsoRAW
	a.update(func() {
		a.importing = true
		a.total, a.done, a.progress, a.result = len(photos), 0, 0, ""
	})
	a.run(func() {
		ctx := context.Background()
		failed := 0
		for i, p := range photos {
			if err := a.importOne(ctx, p, dir, format, quality, alsoRAW); err != nil {
				failed++
			}
			done := i + 1
			a.update(func() {
				a.done = done
				a.progress = float64(done) / float64(len(photos))
			})
		}
		a.update(func() {
			a.importing = false
			a.result = importResult(len(photos)-failed, failed, dir)
		})
	})
}

// importOne writes one photo to dir: the camera's own JPEG, or the JPEG
// converted to the chosen format. The RAW file comes along when asked.
func (a *app) importOne(ctx context.Context, p camera.Photo, dir string, format convert.Format, quality int, alsoRAW bool) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	jpeg, ok := p.JPEG()
	if !ok {
		return fmt.Errorf("%s has no JPEG", p.Title)
	}
	dst := convert.Output(dir, jpeg.Name, format)

	if format == convert.JPEG {
		// Keeping the camera's own file loses nothing.
		if err := a.download(ctx, jpeg.URL, dst); err != nil {
			return err
		}
	} else {
		tmp, err := os.CreateTemp("", "kupic-*.jpg")
		if err != nil {
			return err
		}
		name := tmp.Name()
		tmp.Close()
		defer os.Remove(name)
		if err := a.download(ctx, jpeg.URL, name); err != nil {
			return err
		}
		if err := convert.Run(ctx, name, dst, convert.Options{Format: format, Quality: quality}); err != nil {
			return err
		}
	}

	if alsoRAW {
		if raw, ok := p.RAW(); ok {
			if err := a.download(ctx, raw.URL, filepath.Join(dir, raw.Name)); err != nil {
				return err
			}
		}
	}
	return nil
}

// download streams a file from the source to path.
func (a *app) download(ctx context.Context, url, path string) error {
	body, err := a.src.Open(ctx, url)
	if err != nil {
		return err
	}
	defer body.Close()
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(file, body)
	return err
}

// chooseDir asks where imports should go.
func (a *app) chooseDir() {
	if a.win == nil {
		return
	}
	a.run(func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:    a.win,
			Title:     "Where should imports go?",
			Directory: true,
		})
		if err != nil || len(paths) == 0 {
			return
		}
		a.update(func() {
			a.dir = paths[0]
			a.saveSettings()
		})
	})
}

// importResult says what an import wrote.
func importResult(written, failed int, dir string) string {
	if failed == 0 {
		return fmt.Sprintf("Saved %s to %s", plural(written, "photo"), dir)
	}
	if written == 0 {
		return fmt.Sprintf("Nothing was saved: %s failed", plural(failed, "photo"))
	}
	return fmt.Sprintf("Saved %s; %s failed", plural(written, "photo"), plural(failed, "photo"))
}
