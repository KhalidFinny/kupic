package main

import (
	"bufio"
	"context"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"sync"

	"github.com/egoist/mygo/ui"
	xdraw "golang.org/x/image/draw"

	"github.com/KhalidFinny/kupic/internal/camera"
)

// thumbPixels is the longest side of a thumbnail, enough for the grid at
// any window size and small enough to keep many.
const thumbPixels = 384

// thumbWorkers is how many thumbnails are fetched at once. A thumbnail is a
// few kilobytes, so a handful in flight fills a screen quickly without
// crowding the camera's own access point.
const thumbWorkers = 6

// thumbCacheSize is how many thumbnails are kept. A 384-pixel bitmap is about
// half a megabyte, and the grid draws a few dozen at a time, so this holds
// several screens of scrolling without keeping a whole card decoded.
const thumbCacheSize = 256

// noThumbCacheSize is how many photos are remembered as having no thumbnail.
const noThumbCacheSize = 4096

// thumbCache is the thumbnails the grid has, the photos known to have none,
// and the ones a worker has taken. Every entry is keyed by photo ID, so a new
// card is new keys.
type thumbCache struct {
	mu      sync.Mutex
	bitmaps map[string]*ui.Bitmap
	order   []string // the bitmaps' order, oldest first
	missing map[string]bool
	missed  []string // the missing photos' order, oldest first
	wanting map[string]bool
}

func newThumbCache() thumbCache {
	return thumbCache{
		bitmaps: map[string]*ui.Bitmap{},
		missing: map[string]bool{},
		wanting: map[string]bool{},
	}
}

// thumb returns a photo's thumbnail, whether one can be made at all, and asks
// a worker for it when the grid has neither. The grid calls it for the cells
// it draws, on every frame, so it takes one lock and never blocks.
func (a *app) thumb(photo camera.Photo) (*ui.Bitmap, bool) {
	c := &a.thumbs
	c.mu.Lock()
	bitmap, failed := c.bitmaps[photo.ID], c.missing[photo.ID]
	ask := bitmap == nil && !failed && !c.wanting[photo.ID]
	if ask {
		c.wanting[photo.ID] = true
	}
	c.mu.Unlock()

	if ask {
		select {
		case a.want <- photo:
		default:
			// The queue is full; the next frame asks again.
			c.release(photo.ID)
		}
	}
	return bitmap, failed
}

// release gives up a photo a worker was asked for and did not fetch.
func (c *thumbCache) release(id string) {
	c.mu.Lock()
	delete(c.wanting, id)
	c.mu.Unlock()
}

// forget drops the requests for the card that was here, so the next card's
// workers do not fetch photos that are no longer shown.
func (c *thumbCache) forget() {
	c.mu.Lock()
	clear(c.wanting)
	c.mu.Unlock()
}

// forgetThumbs drops the queued requests and the claim on each of them: the
// workers for the last card have gone, and new ones would fetch photos that
// are no longer shown.
func (a *app) forgetThumbs() {
drain:
	for {
		select {
		case <-a.want:
		default:
			break drain
		}
	}
	a.thumbs.forget()
}

// miss records that a photo has no thumbnail, forgetting the oldest when the
// record is full.
func (c *thumbCache) miss(id string) {
	if c.missing[id] {
		return
	}
	c.missing[id] = true
	c.missed = append(c.missed, id)
	if len(c.missed) > noThumbCacheSize {
		delete(c.missing, c.missed[0])
		c.missed = c.missed[1:]
	}
}

// put records a photo's thumbnail, evicting the oldest past the cap.
func (c *thumbCache) put(id string, bitmap *ui.Bitmap) {
	if _, ok := c.bitmaps[id]; !ok {
		c.order = append(c.order, id)
		if len(c.order) > thumbCacheSize {
			delete(c.bitmaps, c.order[0])
			c.order = c.order[1:]
		}
	}
	c.bitmaps[id] = bitmap
}

// setThumb records a photo's thumbnail, or that it has none, and redraws.
func (a *app) setThumb(id string, bitmap *ui.Bitmap) {
	c := &a.thumbs
	c.mu.Lock()
	if bitmap == nil {
		c.miss(id)
	} else {
		c.put(id, bitmap)
	}
	c.mu.Unlock()
	a.update(func() {})
}

// settleThumbs marks the tiles that never loaded as having no thumbnail:
// nothing more can be fetched from the camera now, so they stop spinning.
func (a *app) settleThumbs() {
	c := &a.thumbs
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, p := range a.photos {
		if c.bitmaps[p.ID] == nil {
			c.miss(p.ID)
		}
	}
}

// startThumbs fetches thumbnails as the grid asks for them. Only the cells the
// grid draws are asked for, so a card of thousands fills one screen instead of
// starting thousands of downloads, which is what made it crawl.
func (a *app) startThumbs(ctx context.Context) {
	for range thumbWorkers {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case photo := <-a.want:
					a.loadThumb(ctx, photo)
				}
			}
		}()
	}
}

// loadThumb fetches one thumbnail and draws it.
func (a *app) loadThumb(ctx context.Context, photo camera.Photo) {
	defer a.thumbs.release(photo.ID)

	file, ok := photo.Thumb()
	if !ok {
		a.setThumb(photo.ID, nil)
		return
	}
	bitmap, err := a.fetchThumb(ctx, file)
	if err != nil {
		// Not every photo has a thumbnail beside it; a slow tile beats an
		// empty one.
		if jpeg, ok := photo.JPEG(); ok && jpeg.URL != file.URL {
			bitmap, err = a.fetchThumb(ctx, jpeg)
		}
	}
	if err != nil {
		// A fetch cut short by a new read says nothing about the photo.
		if ctx.Err() == nil {
			a.setThumb(photo.ID, nil)
		}
		return
	}
	a.setThumb(photo.ID, bitmap)
}

// fetchThumb downloads and shrinks one thumbnail. The decoder reads the body
// as it comes: a copy of the file in memory is no use to it.
func (a *app) fetchThumb(ctx context.Context, file camera.File) (*ui.Bitmap, error) {
	body, err := a.src.Open(ctx, file.URL)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	src, _, err := image.Decode(bufio.NewReader(io.LimitReader(body, 48<<20)))
	if err != nil {
		return nil, err
	}
	return ui.NewBitmap(downscale(src, thumbPixels)), nil
}

// downscale shrinks an image so that its longest side is at most size.
func downscale(src image.Image, size int) image.Image {
	bounds := src.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width <= size && height <= size {
		return src
	}
	if width >= height {
		height, width = height*size/width, size
	} else {
		width, height = width*size/height, size
	}
	dst := image.NewRGBA(image.Rect(0, 0, max(width, 1), max(height, 1)))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, bounds, xdraw.Over, nil)
	return dst
}
