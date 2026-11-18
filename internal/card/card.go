// Package card reads a camera's SD card straight off the drive it appears as
// when the camera is plugged in over USB. A Lumix set to [PC(Storage)] shows
// its card as a USB mass storage volume, so there is no session to open and
// nothing to authenticate: the photos are files on a mounted filesystem.
//
// The same code reads a card in a USB reader, which mounts the same way.
package card

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/KhalidFinny/kupic/internal/camera"
)

// Card is a camera's SD card, mounted as a drive.
type Card struct {
	root  string
	label string
}

// Find returns the camera's card: the first mounted volume that holds a
// Panasonic DCIM folder. The desktop mounts the card on its own when the
// camera is plugged in, so there is nothing to connect to.
func Find() (*Card, error) {
	for _, path := range mounts() {
		if c, err := At(path); err == nil {
			return c, nil
		}
	}
	return nil, fmt.Errorf("no camera card is mounted")
}

// At opens the card at a known root, for a caller that already has one.
func At(root string) (*Card, error) {
	if !hasDCIM(filepath.Join(root, "DCIM")) {
		return nil, fmt.Errorf("%s does not hold a camera card", root)
	}
	return &Card{root: root, label: filepath.Base(root)}, nil
}

// Name is what to call the card: the name of the volume it is mounted as.
func (c *Card) Name() string { return c.label }

// List reads the card's photos, newest first. A photo is a JPEG, with the RAW
// beside it when the camera wrote one.
func (c *Card) List(ctx context.Context) ([]camera.Photo, error) {
	byID := map[string]*camera.Photo{}
	var ids []string
	err := filepath.WalkDir(filepath.Join(c.root, "DCIM"), func(path string, d fs.DirEntry, err error) error {
		// A file that cannot be read, or a walk the caller cancelled, is
		// not worth failing the whole card over.
		if err != nil || d.IsDir() || ctx.Err() != nil {
			return nil
		}
		file, ok := imageFile(path, d)
		if !ok {
			return nil
		}
		// The JPEG and the RAW of one shot share a name, so grouping by
		// the path without its extension puts them on the same photo.
		id := strings.TrimSuffix(path, filepath.Ext(path))
		photo := byID[id]
		if photo == nil {
			photo = &camera.Photo{ID: id, Title: filepath.Base(id)}
			if info, err := d.Info(); err == nil {
				photo.Date = info.ModTime()
			}
			byID[id] = photo
			ids = append(ids, id)
		}
		photo.Files = append(photo.Files, file)
		return nil
	})
	if err != nil {
		return nil, err
	}

	photos := make([]camera.Photo, 0, len(ids))
	for _, id := range ids {
		photos = append(photos, *byID[id])
	}
	sort.SliceStable(photos, func(i, j int) bool { return photos[i].Date.After(photos[j].Date) })
	return photos, nil
}

// Open opens one of the card's files for reading.
func (c *Card) Open(ctx context.Context, path string) (io.ReadCloser, error) {
	return os.Open(path)
}

// imageFile describes a file on the card as one of a photo's files, or reports
// that it is not a photo.
func imageFile(path string, d fs.DirEntry) (camera.File, bool) {
	var mime string
	switch strings.ToUpper(filepath.Ext(path)) {
	case ".JPG", ".JPEG":
		mime = "image/jpeg"
	case ".RW2", ".RAW", ".DNG":
		mime = "image/x-panasonic-rw2"
	default:
		return camera.File{}, false
	}
	info, err := d.Info()
	if err != nil {
		return camera.File{}, false
	}
	return camera.File{URL: path, Name: filepath.Base(path), MIME: mime, Size: info.Size()}, true
}

// hasDCIM reports whether a DCIM folder holds Panasonic photos: a Lumix writes
// into a folder named 100_PANA, 101_PANA and so on.
func hasDCIM(dcim string) bool {
	entries, err := os.ReadDir(dcim)
	if err != nil {
		return false
	}
	for _, e := range entries {
		name := strings.ToUpper(e.Name())
		if e.IsDir() && strings.HasSuffix(name, "_PANA") {
			return true
		}
		if strings.HasPrefix(name, "P") && (strings.HasSuffix(name, ".JPG") || strings.HasSuffix(name, ".RW2")) {
			return true
		}
	}
	return false
}

// cardFilesystems are the filesystems an SD card is formatted with.
var cardFilesystems = map[string]bool{
	"vfat": true, "exfat": true, "msdos": true, "ntfs": true, "ntfs3": true, "fuseblk": true,
}

// mounts lists the mounted volumes that could be a camera card. Only the
// filesystems a card uses are considered, so the walk does not stat every
// network share on the machine.
func mounts() []string {
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return nil
	}
	var out []string
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 || !cardFilesystems[fields[2]] {
			continue
		}
		out = append(out, unescapeMount(fields[1]))
	}
	return out
}

// unescapeMount undoes the escaping /proc/mounts puts in a path.
func unescapeMount(path string) string {
	return strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`).Replace(path)
}
