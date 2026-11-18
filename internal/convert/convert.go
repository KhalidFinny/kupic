// Package convert turns an image into another format with ffmpeg.
package convert

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Format is a format to write an image as.
type Format string

const (
	JPEG Format = "jpeg"
	PNG  Format = "png"
	WebP Format = "webp"
)

// Formats are the formats kupic writes, in the order they are offered.
var Formats = []Format{JPEG, PNG, WebP}

// Label is what a format is called in the interface.
func (f Format) Label() string {
	switch f {
	case JPEG:
		return "JPEG"
	case PNG:
		return "PNG"
	case WebP:
		return "WebP"
	}
	return string(f)
}

// Extension is the file extension a format is written with.
func (f Format) Extension() string {
	switch f {
	case JPEG:
		return ".jpg"
	case PNG:
		return ".png"
	case WebP:
		return ".webp"
	}
	return ".img"
}

// Lossy reports whether a format trades quality for size.
func (f Format) Lossy() bool { return f == JPEG || f == WebP }

// Options say what to make of a source image.
type Options struct {
	Format Format
	// Quality is 1 to 100, for the lossy formats.
	Quality int
	// Width scales the image, keeping its proportions; 0 keeps its size.
	Width int
}

// Run writes src to dst with ffmpeg, which must be on PATH. The directory
// of dst must exist.
func Run(ctx context.Context, src, dst string, o Options) error {
	args := []string{"-hide_banner", "-loglevel", "error", "-y", "-i", src}
	if o.Width > 0 {
		args = append(args, "-vf", "scale="+strconv.Itoa(o.Width)+":-2")
	}
	args = append(args, "-frames:v", "1", "-update", "1")
	switch o.Format {
	case WebP:
		args = append(args, "-c:v", "libwebp", "-quality", strconv.Itoa(clamp(o.Quality)))
	case JPEG:
		args = append(args, "-c:v", "mjpeg", "-q:v", strconv.Itoa(jpegQuality(clamp(o.Quality))))
	case PNG:
		args = append(args, "-c:v", "png", "-compression_level", "6")
	default:
		return fmt.Errorf("cannot write %s", o.Format)
	}
	args = append(args, dst)

	cmd := exec.CommandContext(ctx, ffmpegPath(), args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("ffmpeg: %s", message)
	}
	return nil
}

// Output names the file a source is written as, in dir.
func Output(dir, src string, format Format) string {
	name := strings.TrimSuffix(filepath.Base(src), filepath.Ext(src))
	return filepath.Join(dir, name+format.Extension())
}

// Available reports why conversion cannot run, and nil when it can.
func Available() error {
	if _, err := exec.LookPath(ffmpegPath()); err != nil {
		return fmt.Errorf("ffmpeg is not installed")
	}
	return nil
}

// ffmpegPath is the ffmpeg to run: one next to the app, else the one on
// PATH.
func ffmpegPath() string {
	if exe, err := os.Executable(); err == nil {
		beside := filepath.Join(filepath.Dir(exe), "ffmpeg")
		if info, err := os.Stat(beside); err == nil && !info.IsDir() {
			return beside
		}
	}
	return "ffmpeg"
}

// clamp keeps a quality between 1 and 100, defaulting to 90.
func clamp(quality int) int {
	if quality <= 0 {
		return 90
	}
	if quality > 100 {
		return 100
	}
	return quality
}

// jpegQuality maps a quality of 1 to 100 onto ffmpeg's mjpeg scale, where
// 2 is best and 31 is worst.
func jpegQuality(quality int) int { return 31 - (quality*29)/100 }
