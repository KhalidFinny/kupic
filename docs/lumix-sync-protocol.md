# What LUMIX Sync actually does

Findings from decompiling **LUMIX Sync 2.0.17** (`com.panasonic.jp.lumixsync`,
62.6 MB, `classes.dex` + `classes2.dex`, native `libdlnaCore.so`) with jadx, and
from a survey of the public reverse-engineering ecosystem.

This is the reference for kupic's Wi-Fi and Bluetooth paths. The two things the
world did not have — the `req_acc_e` cipher and the BLE Wi-Fi handover — are
both below.

## 1. The app has no plugins

It is a **monolithic native Android app** — not Flutter, not React Native, no
WebView. There is no plugin, extension, or scripting surface. `libdlnaCore.so`
is Panasonic's own DLNA core; the rest is `com.panasonic.jp.*` Java plus
vendored Google libs. So "find a plugin on it" has no answer: there is nothing
to hook. Anything reusable has to come from the camera protocol itself.

## 2. The official SDK does not help

Panasonic's **LUMIX SDK** (`LumixRemoteControlLibrary`) is Windows 10 x64,
Visual Studio 2015, **USB and Ethernet only**. **LUMIX Tether** is Windows/macOS.
Neither lists the **G100/G100D** — the SDK covers the S series, GH5/GH5S/G9/
G9M2/GH6/GH7 and BGH1/BS1H. The G100D also has **no `PC(Tether)` USB mode**: its
`[USB Mode]` is only `[Select on connection] / [PC(Storage)] / [PC(Webcam)]`, and
Panasonic's own docs say Wi-Fi and Bluetooth are unavailable while tethered. So
there is no SDK path and no PTP tether path for this body.

## 3. The open-source ecosystem

None of these implement the two hard parts. All are useful as references only.

| Project | Lang | What it is | Gap |
|---|---|---|---|
| `gphoto/libgphoto2` `camlibs/lumix` | C (LGPL) | Wi-Fi Lumix driver, curl+libxml2, ships in distros | plain `req_acc` only; capture not working |
| `njfdev/liblumix` | C++ | S5IIX Wi-Fi driver for INDI | tries `req_acc_e` with `hex(udn)`; admits the camera shows a garbage name |
| `abichinger/pylumix` | Python | Wi-Fi control, MIT | plain `req_acc`, hardcoded device id |
| `vvvrrooomm/lumix_control` | C++/Qt | TZ71 remote, live view | older body |
| `peci1/lumix-link-desktop` | Java | Desktop Lumix Link | older protocol |
| `davidkim9/LumixController` | Electron | G7 control | older body |
| `Rambalac/GMaster` | C# | Windows UWP remote | Windows |
| `totoantibes/LumixCameraAscomDriver`, `NinaLumixPlugin` | C#/.NET | astrophotography plugins | Windows, no G100D |
| `auberginehill/panasonic-html-image-app` | JS | browser remote | older protocol |
| `klausweiss/lumix-upnp-dump` | — | dump media over UPnP | listing only |
| `rmoscetti/lumix-g7-vision`, `JYMarz/Lumix_wifi_tether` | Python | G7 / LX-100 | older bodies |
| `tobiasbrummer/lux-lat-long-log` | C++/ESP32 | BLE GPS injection, decoded from an S5 | GPS only, no Wi-Fi |

`libgphoto2` issue #904 has been open since April 2023 with "likely be harder to
reverse engineer how they generate those" — that is the `req_acc_e` cipher,
cracked below.

## 4. The `req_acc_e` cipher — cracked

Newer firmware rejects the plain `req_acc` and wants `req_acc_e` with two
parameters that change every session. Nobody had published how they are made.

**It is not real cryptography.** It is XOR with a 32-bit session key, plus a
byte-swap inside each 4-byte word, printed as hex. The key is handed to the
client in the clear by `req_acc_g`.

### The algorithm

`N5/o.b(str, key)` — port this:

```go
func encode(plain string, key uint32) string {
    // pad to a multiple of 4 with 0xFF
    b := []byte(plain)
    for len(b)%4 != 0 {
        b = append(b, 0xFF)
    }
    var out strings.Builder
    for i := 0; i < len(b); i += 4 {
        w := binary.BigEndian.Uint32(b[i:])
        v := w ^ key
        // swap the bytes within the word, then print as hex
        swapped := bits.RotateLeft32(bits.ReverseBytes32(v), 0)
        fmt.Fprintf(&out, "%08x", swapped)
    }
    return out.String()
}
```

where the swap is `((v>>24)&0xff) | ((v<<24)&0xff000000) | ((v<<8)&0x00ff0000) | ((v>>8)&0x0000ff00)`,
i.e. a full byte-reverse of the 32-bit word. `key == 0` means the app's default
`0x9032008C`.

### The exchange

1. `GET /cam.cgi?mode=accctrl&type=req_acc_g` — the reply is **`text/plain`**,
   the app reads it verbatim and splits on `,`. Field 1, parsed as hex, is the
   session key. (If the read fails the key is `-1` → `encrypt_key_failure`.)
2. `value  = encode("4D454930-0100-1000-8001-" + seed, key)`
   where `seed` is the app's own persisted 12-char id (`Dlna_UUID_Seed`).
3. `value2 = encode(clientName, key)`
4. `GET /cam.cgi?mode=accctrl&type=req_acc_e&value=<value>&value2=<value2>`

The values are **not** URL-encoded in the URL; only the device name is encoded
(and only on the non-encrypted path).

### Verified against real captures

The two samples published in libgphoto2 #904 both decode to the same plaintext,
which confirms the algorithm exactly:

```
plaintext  = 4D454930-0100-1000-8001-02000003DF41
session 1  : key 0x3cff4544  value = 7171bb08…b978   ✓ exact
session 2  : key 0x2e9c255e  value = 6b11d81a…da6a   ✓ exact
```

Re-encoding that plaintext with each recovered key reproduces the captured
ciphertext byte for byte, and XOR-ing the two captures gives a constant
`1a606312` per word — the cipher structure, confirmed.

So a client generates its own seed, asks for the key, encodes, and connects —
no capture, no replay, no user password.

## 5. The Bluetooth side — how Wi-Fi really starts

This is the part the public record was missing entirely, and the reason
`ble/lumix.go` cannot raise Wi-Fi on its own. Verified by decompiling the BLE
stack (`I5/e.java`, `I5/g.java`), the request runner (`K5/d.java`), the
handshake (`K5/j.java`), the codec (`N5/o.java`), the helpers (`N5/l.java`) and
the state classes (`H5/a.java`, `z5/C3143b.java`, `z5/C3145d.java`).

### The claim, confirmed

The camera **hands its own access point's SSID and password to the client over
BLE reads**, after the session writes. It does not just "come up": the app reads
the credentials, tells the camera which mode to use, then joins.

### The read map

Reads go through `I5.e.z(int)` / `I5.g.R0(int)`. All the ones that matter are on
the session service `054ac620-3214-11e6-ac0d-0002a5d5c51b`:

| Type | Characteristic | Meaning |
|---|---|---|
| 5 | `e206a5c0-3214-11e6-afe4-0002a5d5c51b` | **`CAMERA_SSID`** |
| 6 | `c97cf1a5-3c03-4290-8c1b-9e74b9500f54` | **`CAMERA_WIFI_PASSWORD`** |
| 35 | `03e23a31-a54c-40fa-a668-de9acdc910bb` | `INITIAL_INFORM` — the BLE session key |

Others exist (`34` DAC, `36–38` camera settings on `054ac620-3214-11e6-0001-…`,
`15`, `41`, `44`, `45`, `48`, `49`, `54`) but none are needed to get on Wi-Fi.

### The write map

Writes go through `I5.e.K(int, byte[])` / `I5.g.q1`/`r1`.

| Type | Service | Characteristic | Meaning |
|---|---|---|---|
| 1 | `054ac620-…-ac0d-…` | `8d08a420-3213-11e6-8aca-…` | `DLNA_UUID` — the `MEI0` write |
| 2 | `054ac620-…-ac0d-…` | `cd7a71a0-3213-11e6-8f56-…` | `FRIENDLY_NAME` |
| 11 | `5ae57260-3214-11e6-979e-…` | `0d6f1880-3217-11e6-a4cb-…` | **Wi-Fi mode** (`{1}` SoftAP / `{2}` STA) |
| 12 | `5ae57260-…-979e-…` | `18345be0-3217-11e6-b56c-…` | SSID for STA mode (32-byte padded) |
| 16 | `054ac620-…-ac0d-…` | `bb5d2a8c-5d3b-43ad-bfc3-470cad55c1ca` | `DLNA_UUID_APS` |
| 17 | `5ae57260-…-979e-…` | `7b61774f-efb4-44c6-a66d-c69e3b384c4a` | `FRIENDLY_NAME_APS` |
| 7 | `1d74afe0-…` | `daff1bc0-3216-11e6-91c8-…` | GPS |
| 9 | `34738720-…` | `ead55e60-3216-11e6-a42e-…` | clock |
| 39 | `7be5faae-…` | `7be5fd56-…` | keep-alive |

(Service assignments for 16/17 are cross-referenced from the services-discovered
code, which looks `bb5d2a8c` up under `054ac620-…-ac0d-…`; jadx hoisted a couple
of assignments in the write map, so treat that column as the reliable one.)

### The BLE writes are encrypted too

Every payload is run through the same XOR cipher as `req_acc_e`, with the key
read from `INITIAL_INFORM`:

- `key = bigEndianUint32(first 4 bytes of read 35)` (`z5.C3143b.e`)
- string → 20 bytes: `N5.o.c(str, key, 20)` — used for `FRIENDLY_NAME_APS`
- bytes → 20 bytes: `N5.o.d(bytes, key, 20)` — used for `DLNA_UUID_APS`
- decode: `N5.o.a(bytes, key)` — used for the password

The key is `0` (→ the app's default `0x9032008C`) until `INITIAL_INFORM` arrives.

### The exact handshake (`K5/j.java`)

```
connect → discoverServices
if the camera exposes bb5d2a8c-… (the APS characteristic):   // g.f2708B
    read  INITIAL_INFORM (35)          → key
    write DLNA_UUID_APS  (16) = o.d(dlnaUUID, key, 20)
    write FRIENDLY_NAME_APS (17) = o.c(clientName, key, 20)
else:
    write DLNA_UUID      (1)  = MEI0 packet
    write FRIENDLY_NAME  (2)  = clientName
read  CAMERA_SSID          (5)  → ssid      (NUL-terminated ASCII)
read  CAMERA_WIFI_PASSWORD (6)  → password  (o.a(bytes, key).trim())
```

Then the app decides the mode from the phone's own network state:

```
if the phone is not on Wi-Fi (or its IP is 0.0.0.0):
    write Wi-Fi mode (11) = {1}   // SoftAP: the camera starts its own AP
else:
    write Wi-Fi mode (11) = {2}   // STA: the camera joins a network
    write SSID       (12) = pad32(phoneSSID)
```

and finally joins the camera's AP with the password it just read
(`M5/d.java`: scan up to 5×5 s for the SSID to appear, then
`requestNetwork` with `setWpa2Passphrase`/`setWpa3Passphrase`, waiting up to
90 s).

The two values are kept as `CurrentConnectedSSID` and `CurrentConnectedPass`.

### `MEI0` is not what the reference project thought

The `MEI0` write is not a magic constant with a "counter and CRC that the camera
does not check". It is:

```
MEI0 = hexDecode("4D454930010010008001" + seed)
```

`N5/l.A(context)` builds that 32-character hex string and `N5/l.k` decodes it, so
the 16-byte packet is 10 fixed bytes plus the app's own 6-byte **`Dlna_UUID_Seed`**
(stored once, generated by the DLNA core). That is exactly why the captured
packet ends in `0b54` and another install's ends in `DF41` — the reference
project read those as a counter and a checksum, but they are the client's
per-install identity. Our hardcoded packet is one valid seed, which is why it
works; a real client should generate its own.

## 6. What this means for kupic

- **USB** (done) needs none of this: the card is a mounted filesystem.
- **Wi-Fi on the G100D takes the plain `req_acc`.** Measured on the camera:

  ```
  getstate                    → err_reject
  req_acc_g (every shape)     → err_param     (does not exist on this firmware)
  req_acc  (plain)            → ok,G100D-3D5833,remote,encrypted
  req_acc_e                   → err_param
  ```

  So the cipher in §4 is **not needed on this body** — it is the fallback for
  firmware that refuses the plain request. Two things to get right:
  - the trailing `encrypted` in the `ok` line is a **capability flag** ("I can do
    encrypted"), not a demand; reading it as a demand is a mistake we made;
  - `req_acc_e` must not be sent before `req_acc`. Doing so leaves the camera
    answering `ok_under_research_no_msg` to the plain request instead of `ok`,
    which looks exactly like a refusal.
- **Bluetooth** can be made seamless by adding the two reads. Until then the
  user can read the SSID and password off the camera screen.

The §4 cipher is verified against real S5M2 captures, so it is correct — it is
just not what this camera asks for.
