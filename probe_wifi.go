package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/KhalidFinny/kupic/internal/camera"
)

// wifiProbe prints the camera's raw answers to the access handshake:
//
//	kupic wifi            # writes ~/kupic-wifi.txt and prints it
//	kupic wifi -o out.txt
//
// The app does this itself when a connection fails, so this is for running it
// deliberately — the same report, without opening the window.
func wifiProbe() {
	flags := flag.NewFlagSet("wifi", flag.ExitOnError)
	out := flags.String("o", "", "write the report here (default ~/kupic-wifi.txt)")
	// os.Args[1] is "wifi"; the flags come after it.
	if err := flags.Parse(os.Args[2:]); err != nil {
		return
	}

	ctx := context.Background()
	fmt.Println("kupic wifi — asking a Lumix for its raw access replies")

	devices, err := camera.Discover(ctx, 6*time.Second)
	if err != nil {
		fmt.Println("discovery failed:", err)
		os.Exit(1)
	}
	if len(devices) == 0 {
		fmt.Println("\nNo camera answered. On the camera, open")
		fmt.Println("[Wi-Fi] → [Wi-Fi Function] → [New Connection] → [Remote Shooting & View].")
		fmt.Println("Then run this again while this machine is on the camera's own network.")
		os.Exit(2)
	}

	for _, d := range devices {
		report, ok := camera.Diagnose(ctx, d)
		if !ok {
			fmt.Printf("\n%s did not answer; nothing to diagnose.\n", d.IP)
			continue
		}
		fmt.Print("\n", report)
		writeReport(*out, report)
	}
}

// writeReport saves the report where it can be read after switching networks
// back, and says where.
func writeReport(path, report string) {
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		path = filepath.Join(home, "kupic-wifi.txt")
	}
	if err := os.WriteFile(path, []byte(report), 0o644); err != nil {
		fmt.Println("could not write the report:", err)
		return
	}
	fmt.Println("\nsaved to", path)
}
