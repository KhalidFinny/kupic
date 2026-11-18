package main

import "github.com/egoist/mygo/ui"

// The Leica look, as Leica LUX 2.0 and Leica FOTOS draw their galleries:
// black chrome, near-white text, tonal greys, and one red accent used
// sparingly. No glass, no gradients.
var (
	bg        = ui.Hex("#0B0B0C") // the window
	surface   = ui.Hex("#121214") // wells behind photos
	raised    = ui.Hex("#1B1B1E") // bars, chips and cards
	hover     = ui.Hex("#232327")
	pressed   = ui.Hex("#2B2B30")
	line      = ui.Hex("#2A2A2F") // borders
	text      = ui.Hex("#F4F4F2")
	muted     = ui.Hex("#9A9AA2")
	white     = ui.Hex("#FFFFFF")
	accent    = ui.Hex("#E20612") // the Leica red
	accentHi  = ui.Hex("#FF1A26")
	accentLow = ui.Hex("#B90510")
	ok        = ui.Hex("#3FB950")
	warn      = ui.Hex("#E3B341")
	bad       = ui.Hex("#F85149")
)

// leicaTheme is the theme every widget takes its colors and metrics from.
// It stays dark whatever the desktop is: the Leica apps are black.
func leicaTheme() *ui.Theme {
	return &ui.Theme{
		Dark:           true,
		Background:     bg,
		Surface:        raised,
		SurfaceHover:   hover,
		SurfacePressed: pressed,
		Border:         line,
		Text:           text,
		TextMuted:      muted,
		Accent:         accent,
		AccentHover:    accentHi,
		AccentPressed:  accentLow,
		AccentText:     white,
		Danger:         bad,
		Warning:        warn,
		Success:        ok,
		Selection:      ui.RGBA(226, 6, 18, 0.35),
		Focus:          accent,
		Inverse:        ui.Hex("#2E2E33"),
		InverseText:    text,
		Scrollbar:      ui.Hex("#3A3A40"),
		ScrollbarWidth: 8,
		Radius:         6,
		Spacing:        4,
		FontSize:       13,
	}
}
