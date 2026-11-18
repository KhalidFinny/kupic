package main

import "github.com/egoist/mygo/ui"

// The icons the interface draws, as SVGs in the color of the text around
// them.
var (
	iconCamera    = icon(`<path d="M3 8.6A2.6 2.6 0 0 1 5.6 6h1.6l1.3-2h6.9l1.3 2h1.7A2.6 2.6 0 0 1 21 8.6v7.8A2.6 2.6 0 0 1 18.4 19H5.6A2.6 2.6 0 0 1 3 16.4z"/><circle cx="12" cy="12.4" r="3.3"/>`)
	iconRefresh   = icon(`<path d="M20 12a8 8 0 1 1-2.6-5.9"/><path d="M20 4v6h-6"/>`)
	iconCheck     = icon(`<path d="M5 12.6l4.2 4.2L19 7"/>`)
	iconDownload  = icon(`<path d="M12 3.5v11"/><path d="M7.5 10.5L12 15l4.5-4.5"/><path d="M4.5 19.5h15"/>`)
	iconFolder    = icon(`<path d="M3 7.2A2.2 2.2 0 0 1 5.2 5h3.4l1.8 2.2h8.4A2.2 2.2 0 0 1 21 9.4v7.4A2.2 2.2 0 0 1 18.8 19H5.2A2.2 2.2 0 0 1 3 16.8z"/>`)
	iconImage     = icon(`<rect x="3" y="4.5" width="18" height="15" rx="2.4"/><circle cx="8.8" cy="9.6" r="1.6"/><path d="M21 15.5l-5.2-4.6-4 3.6-2.8-2.4L3 17"/>`)
	iconBluetooth = icon(`<path d="M7 7.5l10 9-5 4.5V3l5 4.5-10 9"/>`)
	iconUSB       = icon(`<path d="M9 3.5v4.5"/><path d="M15 3.5v4.5"/><path d="M7 8h10v3.2a5 5 0 0 1-5 5v4.3"/>`)
)

// markAperture is the logo: six shutter blades between two hexagons, the
// opening red. It is not an interface icon, so it carries its own stroke.
var markAperture = ui.MustParseSVG([]byte(`<svg viewBox="0 0 96 96" fill="none" ` +
	`stroke="currentColor" stroke-width="4.6" stroke-linecap="round" stroke-linejoin="round">` +
	`<polygon points="82,48 65,77.44 31,77.44 14,48 31,18.56 65,18.56"/>` +
	`<polygon points="62.72,56.5 48,65 33.28,56.5 33.28,39.5 48,31 62.72,39.5"/>` +
	`<path d="M62.72 56.5L65 77.44M48 65L31 77.44M33.28 56.5L14 48M33.28 39.5L31 18.56M48 31L65 18.56M62.72 39.5L82 48"/>` +
	`<circle cx="48" cy="48" r="5.4" fill="#E20612" stroke="none"/></svg>`))

// icon wraps SVG body in a 24-unit stroked frame.
func icon(body string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" ` +
		`stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">` + body + `</svg>`))
}
