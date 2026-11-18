package camera

import (
	"context"
	"encoding/binary"
	"fmt"
	"math/bits"
	"net/url"
	"strconv"
	"strings"
)

// defaultSessionKey is the key LUMIX Sync uses when the camera reports none.
const defaultSessionKey uint32 = 0x9032008C

// clientSeed is the tail of this client's identity. The same six bytes end the
// BLE MEI0 packet (ble/lumix.go), so both paths present one client to the
// camera. The camera only uses it as a name, so any six bytes do.
const clientSeed = "020000030b54"

// ClientUDN is the identity this client presents when it asks for access.
func ClientUDN() string { return "4D454930-0100-1000-8001-" + clientSeed }

// sessionKey asks the camera which key it wants the access request encoded
// with. Newer firmware answers req_acc_g in plain text as "<status>,<key in
// hex>". A camera that answers something else leaves 0, and the default key is
// used instead; the raw answer comes back too, because that is the only way to
// see a reply format this does not recognise.
func (c *Client) sessionKey(ctx context.Context) (uint32, string) {
	body, err := c.camcgi(ctx, url.Values{"mode": {"accctrl"}, "type": {"req_acc_g"}})
	if err != nil {
		return 0, err.Error()
	}
	raw := strings.TrimSpace(string(body))
	fields := strings.Split(raw, ",")
	if len(fields) < 2 {
		return 0, raw
	}
	key, err := strconv.ParseUint(strings.TrimSpace(fields[1]), 16, 32)
	if err != nil {
		return 0, raw
	}
	return uint32(key), raw
}

// encodeSession encodes a value for req_acc_e: each 32-bit word is XORed with
// the session key and its bytes reversed, then written as hex. The camera hands
// the key over in the clear, so this is per-session obfuscation, not a secret.
//
// See docs/lumix-sync-protocol.md §4.
func encodeSession(value string, key uint32) string {
	if key == 0 {
		key = defaultSessionKey
	}
	raw := []byte(value)
	for len(raw)%4 != 0 {
		raw = append(raw, 0xFF)
	}
	var out strings.Builder
	for i := 0; i < len(raw); i += 4 {
		word := binary.BigEndian.Uint32(raw[i:])
		fmt.Fprintf(&out, "%08x", bits.ReverseBytes32(word^key))
	}
	return out.String()
}
