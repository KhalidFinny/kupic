# kupic — Project Conventions

A native desktop and phone app that pulls photos off a camera that brings its
own Wi-Fi, over USB from the card it mounts as or over that network, without
the maker's app. Built with [MyGo](https://mygo.egoist.dev/) native UI.

**What it is for, and what it is not.** The category is any camera that serves
its card over its own Wi-Fi. The implementation is Panasonic's: kupic is
written and tested against a Lumix G100D, and `internal/camera` speaks
`cam.cgi`. Do not claim another maker in the README or the interface until it
has been run against that camera. Adding one means a second implementation
behind `camera.Client`, which is why the protocol stays isolated in one
package.

## What kupic is (do not re-derive)

- **USB first, Wi-Fi second, Bluetooth third.** The simplest path is the cable:
  with `[USB Mode]` set to `[PC(Storage)]` the camera shows its SD card as a
  plain USB mass storage volume, and `package card` reads the `DCIM` folder off
  the drive it mounts as. No session, no authentication, no Bluetooth, no Wi-Fi
  — and a card in a USB reader is the same code path. Wi-Fi and Bluetooth
  remain, as alternatives, for a camera across the room.
- **Wi-Fi for the transfer, Bluetooth to register and wake.** The camera is a
  small web server on the network; kupic finds it with SSDP, opens a session
  through its `cam.cgi` control interface, lists the card over its UPnP
  content directory, and downloads the files.
- **The camera is in [Direct] mode by default: it starts its own Wi-Fi
  network, and the machine joins it.** This is the flow LUMIX Sync uses, and
  it is the normal case, not a fallback. The camera answers at
  `192.168.54.1` on that network. `package wifi` joins it through
  NetworkManager (`nmcli`); if the camera is instead set to `[Via Network]`
  it appears on this machine's own network and SSDP finds it directly.
- **Pairing happens in the app, like LUMIX Sync.** The user presses *Find
  cameras*; kupic scans over Bluetooth, lists the Lumix cameras it finds, and
  the user pairs one. Pairing connects over Bluetooth and runs the app's
  handshake, which registers kupic on the camera and makes it bring its Wi-Fi
  up. Pairing through the laptop's own Bluetooth settings is not the path, and
  is not expected to work.
- **A camera is remembered by name, never by address.** The camera advertises
  a resolvable private address (RPA) that rotates every ~15 minutes, so its
  Bluetooth address is useless as an identity. `settings.Camera` holds the
  name, and a scan prefers it by name.
- **The Bluetooth handshake needs no pairing or bonding.** A Lumix's ATT
  writes are plaintext; the "pairing" is application-layer (the `MEI0`
  session write). See `ble/lumix.go`; the protocol comes from
  github.com/tobiasbrummer/lux-lat-long-log, decoded from a LUMIX S5.
- **Desktop first, the phone alongside it.** MyGo v0.10.0 and later build
  the same Go app for Android (and iOS), so the phone is one binary again.
  The two shells differ: `view.go` is the desktop window, with its own
  title bar and a toolbar; `view_mobile.go` is the compact shell a phone
  or a narrow window gets, with the actions along the bottom where a thumb
  reaches and no title bar to drag. `app.compact` is the switch, set from
  the window's size in `view`. The hardware paths are still desktop-only:
  `/proc/mounts`, `nmcli` and BlueZ do not exist on a phone, so a phone
  build opens and lays out but finds no camera yet.
- **Not connected to photoby.** The user rejected that; kupic keeps a local
  library and nothing else.

## Architecture (do not re-derive)

- **Native UI only.** No webview, no frontend, no Bun. `WindowOptions.Content`
  with `ui.View`. An app of native UI needs GTK 3 on Linux and nothing else.
- **The camera protocol is undocumented and per-model.** It follows the
  reverse-engineering of `cleverfox/lumixproto`, `njfdev/liblumix` and
  `palmdalian/python_lumix_control`. Keep it isolated in `package camera`,
  behind `Client`, so a protocol change is one package.
- **Bluetooth goes through BlueZ over D-Bus** (`godbus`), never `bluetoothctl`
  and never a cgo binding. It lives in `package ble`, behind `Central` and
  `Conn`, so a BlueZ change is one package. Two BlueZ rules the code depends
  on: **discovery must be stopped before connecting** (the controller aborts a
  connection made while scanning), and **`ServicesResolved` must be waited
  for** before looking up a characteristic (BlueZ discovers the GATT database
  after the connection, and an early lookup reports an empty device).
- **Wi-Fi joins go through NetworkManager** (`nmcli`), in `package wifi`.
  Never `wpa_supplicant` directly, never a cgo binding. `nmcli -t` output is
  parsed, so escaped colons in an SSID are unescaped.
- **Two ports matter:** the camera answers `cam.cgi` on **port 80** and its
  UPnP server (device description and content directory) on **port 60606**.
- **Conversion shells out to `ffmpeg`.** No cgo, no image library for
  encoding. `ffmpeg` is expected on `PATH`, or next to the executable.
- **Long work runs off the main thread** and publishes through
  `Window.Update`, which is asynchronous. The view never blocks.

## Repo layout

```
main.go          the window, and the `probe` subcommand
app.go           the app state, its phases, and the photo source interface
view.go          the shell: title bar, toolbar, body, action bar, dialog
photos.go        the photo grid
camera_work.go   discovery, Bluetooth wake, connection, thumbnails, USB read
import.go        downloading and converting the chosen photos
probe.go         the Bluetooth check, against a real camera
theme.go         the Leica palette and theme
icons.go         the SVGs
settings.go      what the app remembers between runs
internal/card/     the SD card as a mounted drive, over USB
internal/camera/   the Lumix protocol: SSDP, cam.cgi, UPnP content directory
internal/ble/      BlueZ over D-Bus, and the Lumix BLE handshake
internal/wifi/     joining the camera's own access point, through NetworkManager
internal/convert/  ffmpeg wrappers
docs/            the LUMIX Sync protocol reference, and plans
```

## Conventions

- **Module:** `github.com/KhalidFinny/kupic`. MyGo is a local `replace` to
  `/home/finny/projects/mygo` until it is released at the pinned version.
- **Files stay under ~500 lines.** Split by responsibility.
- **The root is the app; `internal/` is everything it is built on.** As
  meiro does: the window, its views and its features are flat files at the
  root, one per feature, and the packages behind them (`card`, `camera`,
  `ble`, `wifi`, `convert`) live under `internal/` so no other module can
  import them. Do not add a `views/` or `ui/` folder: Go would force the
  app's state to be exported to cross a package boundary, which is worse
  than a flat root.
- **Views are per feature, and shared views do not render themselves.** A
  file owns one feature (`discover.go`, `import_dialog.go`, `thumbs.go`),
  and a shell (`view.go`, `view_mobile.go`) composes them. Anything both
  shells show is a plain function in `components.go` that takes the data it
  shows plus callbacks and returns the element; it reads no `*app` and
  writes no state. State changes are the app's own action methods
  (`setFormat`, `tapPhoto`, `closeImport`), never inside a builder.
- **No comment slop.** Comments explain a non-obvious *why*, usually one line.
- **Do not wrap everything in try/catch** — handle errors at I/O boundaries.
- **UI direction comes from Khalid.** The look is Leica LUX 2.0 / Leica
  FOTOS: black chrome, near-white text, tonal greys, one red accent used
  sparingly, small radii, no glass, no gradients, no text under 12px. See
  `theme.go`. Do not invent a palette.
- **Handoff and progress** live at the repo root and are gitignored. Keep
  them current.

## Commands

```bash
export PATH="$HOME/.local/go/bin:$PATH"   # go is not on PATH by default
go build ./...
go vet ./...
go tool mygo dev          # run with live reload
go tool mygo build        # package into build/
just snapshots            # render the pages to PNG, for looking at them
./build/linux-amd64/kupic probe   # check the Bluetooth path against a camera
```

## Working agreement

- Do not run unit/smoke/e2e tests unless asked. `build`, `vet` and
  `snapshots` are allowed. The snapshot test is how a UI change is shown.
- Do not deploy or push without an explicit command.
- `kupic probe` is the only way to verify the Bluetooth path, and it needs a
  real camera. It was last run against the G100D: the camera advertises the
  right services and now accepts a connection, but it went out of range before
  the handshake could be confirmed (see `progress.md`).
