package main

import (
	"log"
	"os"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/KhalidFinny/kupic/internal/wifi"
)

func main() {
	// `kupic probe` checks the Bluetooth path against a real camera, and
	// `kupic wifi` prints the camera's raw access replies. The window can do
	// neither on its own.
	for _, arg := range os.Args[1:] {
		if arg == "probe" {
			probe()
			return
		}
		if arg == "wifi" {
			wifiProbe()
			return
		}
	}

	a := newApp()
	mygo.App.WhenReady(func() {
		a.win = mygo.NewWindow(mygo.WindowOptions{
			Title:           "kupic",
			Width:           1180,
			Height:          760,
			MinWidth:        860,
			MinHeight:       540,
			TitleBarStyle:   mygo.TitleBarHidden,
			BackgroundColor: "#0B0B0C",
			StateKey:        "main",
			Content:         ui.View(a.view),
		})
	})
	err := mygo.App.Run()
	// Closing the window should not leave the machine on the camera's access
	// point, which has no internet. There is no window to report to now, so
	// this waits rather than running in the background.
	if a.joined != "" {
		if joinErr := wifi.Return(a.home); joinErr != nil {
			log.Println(joinErr)
		}
	}
	if err != nil {
		log.Fatal(err)
	}
}
