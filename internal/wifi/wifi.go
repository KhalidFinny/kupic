// Package wifi joins the machine to the camera's own access point, which is
// how LUMIX Sync reaches a camera set to [Direct]: the camera starts a Wi-Fi
// network, and the client joins it. The camera answers at Address on it.
//
// It drives NetworkManager through nmcli, its non-interactive interface, so
// the app needs no privileges of its own and no cgo.
package wifi

import (
	"fmt"
	"os/exec"
	"strings"
)

// Address is the camera's address on its own access point.
const Address = "192.168.54.1"

// namePrefixes are how a Lumix camera names its access point, as it names
// itself over Bluetooth.
var namePrefixes = []string{"G100", "G110", "GH", "G9", "S1", "S9", "DC-", "LUMIX"}

// Rescan asks NetworkManager to look for networks again. It is best effort:
// a scan too soon after the last one is refused, which does not matter.
func Rescan() {
	run("device", "wifi", "rescan")
}

// Visible returns the SSIDs of the access points in range.
func Visible() ([]string, error) {
	out, err := output("-t", "-f", "SSID", "device", "wifi", "list", "--rescan", "yes")
	if err != nil {
		return nil, err
	}
	var ssids []string
	for _, line := range strings.Split(out, "\n") {
		ssid := unescape(strings.TrimSpace(line))
		if ssid != "" {
			ssids = append(ssids, ssid)
		}
	}
	return ssids, nil
}

// CameraAP returns the SSID of a visible access point that belongs to a
// Lumix camera, preferring one whose name is the registered camera's.
func CameraAP(camera string) (string, error) {
	ssids, err := Visible()
	if err != nil {
		return "", err
	}
	for _, ssid := range ssids {
		if camera != "" && strings.EqualFold(ssid, camera) {
			return ssid, nil
		}
	}
	for _, ssid := range ssids {
		for _, prefix := range namePrefixes {
			if strings.HasPrefix(strings.ToUpper(ssid), prefix) {
				return ssid, nil
			}
		}
	}
	return "", fmt.Errorf("the camera's own network is not in range")
}

// Join connects this machine to ssid. An empty password joins an open
// network, which is how a Lumix camera's access point starts.
func Join(ssid, password string) error {
	args := []string{"device", "wifi", "connect", ssid}
	if password != "" {
		args = append(args, "password", password)
	}
	if _, err := output(args...); err != nil {
		if password == "" {
			return fmt.Errorf("could not join %s: %w (the camera may ask for a password)", ssid, err)
		}
		return fmt.Errorf("could not join %s: %w", ssid, err)
	}
	return nil
}

// Active returns the SSID this machine is connected to, if any.
func Active() (string, error) {
	out, err := output("-t", "-f", "ACTIVE,SSID", "device", "wifi")
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Split(line, ":")
		if len(fields) >= 2 && fields[0] == "yes" {
			return unescape(fields[1]), nil
		}
	}
	return "", nil
}

// Return reconnects this machine to a network it was on before. The camera's
// own access point has no uplink, so joining it costs the internet until this
// puts the machine back.
func Return(ssid string) error {
	if ssid == "" || ConnectedTo(ssid) {
		return nil
	}
	// The saved profile is what carries the password; a network that was
	// never saved needs its password again, which only the user has.
	if _, err := output("connection", "up", ssid); err == nil {
		return nil
	}
	if _, err := output("device", "wifi", "connect", ssid); err != nil {
		return fmt.Errorf("could not return to %s: %w", ssid, err)
	}
	return nil
}

// ConnectedTo reports whether the machine is already on ssid.
func ConnectedTo(ssid string) bool {
	active, err := Active()
	return err == nil && active != "" && strings.EqualFold(active, ssid)
}

// output runs nmcli and returns its stdout, with a trimmed error.
func output(args ...string) (string, error) {
	cmd := exec.Command("nmcli", args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return "", fmt.Errorf("%s", firstLine(message))
	}
	return string(out), nil
}

// run runs nmcli for its effect, ignoring what it answers.
func run(args ...string) { output(args...) }

// unescape undoes nmcli's escaping of a value in terse output.
func unescape(value string) string {
	value = strings.ReplaceAll(value, `\:`, ":")
	return strings.ReplaceAll(value, `\\`, `\`)
}

// firstLine trims a message to one line.
func firstLine(text string) string {
	line, _, _ := strings.Cut(text, "\n")
	return strings.TrimSpace(line)
}
