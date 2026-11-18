package main

import (
	"github.com/egoist/mygo/ui"

	"github.com/KhalidFinny/kupic/internal/camera"
)

// The grid's metrics, in DIPs. A phone fits three across: at the desktop's
// cell width it would show one photo per screen.
const (
	gridMinWidth = 186
	gridHeight   = 186
	gridGap      = 14
	gridPadding  = 22
	compactCell  = 104
	compactCellH = 118
	compactGap   = 5
	compactPad   = 12
)

// photoGrid shows the card's photos. The grid builds only the cells in
// view, so a card with thousands of them opens at once.
func (a *app) photoGrid(c *ui.Context) {
	if a.compact {
		ui.GridView(c, &a.grid, len(a.photos), compactCell, compactCellH, func(i int) {
			a.cell(c, i)
		}).Grow(1).Padding(compactPad, compactPad, compactPad, compactPad).Gap(compactGap)
		return
	}
	ui.GridView(c, &a.grid, len(a.photos), gridMinWidth, gridHeight, func(i int) {
		a.cell(c, i)
	}).Grow(1).Padding(gridPadding, gridPadding, gridPadding, gridPadding).Gap(gridGap)
}

// cell is one photo in the grid: its thumbnail, a mark when it is chosen,
// and its name while the pointer is on it.
func (a *app) cell(c *ui.Context, i int) {
	photo := a.photos[i]
	chosen := a.chosen.Has(photo.ID)

	cell := ui.Box(c).Fill().Radius(10).Clip().Background(surface)
	if a.selecting {
		cell.OnClick(func() { a.tapPhoto(i) })
	}
	hovered := cell.Hovered()
	switch {
	case chosen:
		cell.Border(2, accent)
	case hovered:
		cell.Border(1, line)
	}
	cell.Children(func() {
		a.thumbnail(c, photo)
		if chosen {
			ui.Box(c).Absolute().Top(9).Right(9).Size(22, 22).Radius(999).Background(accent).Center().
				Children(func() { ui.Icon(c, iconCheck).FontSize(14).TextColor(white) })
		}
		if hovered || chosen {
			ui.Box(c).Absolute().Left(0).Right(0).Bottom(0).Padding(7, 10).
				Background(ui.RGBA(0, 0, 0, 0.62)).Children(func() {
				ui.Text(c, photo.Title).FontSize(12).TextColor(text).SingleLine()
			})
		}
	})
}

// tapPhoto adds a photo to the selection or takes it out. It is what a tap
// does in selection mode, on a phone and a desktop alike.
func (a *app) tapPhoto(i int) {
	id := a.photos[i].ID
	a.selected = i
	if a.chosen.Has(id) {
		a.chosen.Remove(id)
	} else {
		a.chosen.Add(id)
	}
	if a.chosen.Len() == 0 {
		a.selecting = false
	}
}

// thumbnail shows a photo's thumbnail, a spinner while it loads, or a mark
// when the camera offers none. It asks for the thumbnail here, so only the
// photos the grid draws are fetched.
func (a *app) thumbnail(c *ui.Context, photo camera.Photo) {
	bitmap, failed := a.thumb(photo)
	switch {
	case bitmap != nil:
		ui.Image(c, bitmap).Fill().Fit(ui.Cover)
	case failed:
		ui.Box(c).Fill().Background(surface).Center().Children(func() {
			ui.Icon(c, iconImage).FontSize(28).TextColor(line)
		})
	default:
		ui.Box(c).Fill().Background(surface).Center().Children(func() {
			ui.Spinner(c)
		})
	}
}
