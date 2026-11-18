package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"

	"github.com/KhalidFinny/kupic/internal/convert"
)

// settings is what the app remembers between runs.
type settings struct {
	Dir     string `json:"dir"`
	Format  string `json:"format"`
	Quality int    `json:"quality"`
	AlsoRAW bool   `json:"alsoRaw"`
	// Camera is the registered camera, by name: its Bluetooth address
	// rotates, so the name is what lasts between runs.
	Camera string `json:"camera"`
	// WifiPassword is the camera's own network's password, empty when the
	// network is open.
	WifiPassword string `json:"wifiPassword"`
}

// settingsPath is where the settings live.
func settingsPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "kupic", "settings.json")
}

// picturesDir is where imports go unless the user chooses another: the
// platform's own pictures folder, which is XDG_PICTURES_DIR on Linux and the
// app's storage on a phone, with a kupic folder inside it. It is created on
// first use, so an import has somewhere to write.
func picturesDir() string {
	dir, err := mygo.App.Path(mygo.PathPictures)
	if err != nil || dir == "" {
		home, homeErr := os.UserHomeDir()
		if homeErr != nil {
			return "kupic"
		}
		dir = filepath.Join(home, "Pictures")
	}
	return filepath.Join(dir, "kupic")
}

// loadSettings reads what the last run remembered, if anything.
func (a *app) loadSettings() {
	path := settingsPath()
	if path == "" {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var s settings
	if err := json.Unmarshal(data, &s); err != nil {
		return
	}
	if s.Dir != "" {
		a.dir = s.Dir
	}
	if s.Format != "" {
		a.format = convert.Format(s.Format)
	}
	if s.Quality > 0 {
		a.quality = s.Quality
	}
	a.alsoRAW = s.AlsoRAW
	a.camera = s.Camera
	a.wifiPassword = s.WifiPassword
}

// saveSettings writes what this run chose.
func (a *app) saveSettings() {
	path := settingsPath()
	if path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	data, err := json.MarshalIndent(settings{
		Dir: a.dir, Format: string(a.format), Quality: a.quality, AlsoRAW: a.alsoRAW,
		Camera: a.camera, WifiPassword: a.wifiPassword,
	}, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0o644)
}

// plural writes a count with its noun, in the right number.
func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
