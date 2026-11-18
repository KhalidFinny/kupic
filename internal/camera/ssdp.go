// Package camera talks to a Panasonic Lumix camera over Wi-Fi: it finds one
// with SSDP, opens a session through the camera's cam.cgi control interface,
// lists the card over its UPnP content directory, and downloads the files.
//
// The protocol is undocumented. It follows the reverse-engineering of
// cleverfox/lumixproto, njfdev/liblumix and palmdalian/python_lumix_control.
package camera

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
)

// The address SSDP listens on and the service a Lumix camera answers for.
const (
	ssdpAddr = "239.255.255.250:1900"
	cdsST    = "urn:schemas-upnp-org:service:ContentDirectory:1"
)

// Device is a camera found on the network.
type Device struct {
	// IP is the camera's address.
	IP string
	// Location is the URL of the camera's UPnP device description, whose
	// path carries a per-camera segment that the handshake needs.
	Location string
}

// Discover finds Lumix cameras on the local network. It sends SSDP searches
// and waits up to wait for the cameras to answer, returning each once.
func Discover(ctx context.Context, wait time.Duration) ([]Device, error) {
	conn, err := net.ListenPacket("udp4", ":0")
	if err != nil {
		return nil, fmt.Errorf("open the discovery socket: %w", err)
	}
	defer conn.Close()

	dst, err := net.ResolveUDPAddr("udp4", ssdpAddr)
	if err != nil {
		return nil, err
	}
	search := "M-SEARCH * HTTP/1.1\r\n" +
		"HOST: " + ssdpAddr + "\r\n" +
		"MAN: \"ssdp:discover\"\r\n" +
		"MX: 1\r\n" +
		"ST: " + cdsST + "\r\n\r\n"

	deadline := time.Now().Add(wait)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return nil, err
	}

	// Ask three times: a datagram can be lost on a busy network.
	for i := 0; i < 3; i++ {
		if _, err := conn.WriteTo([]byte(search), dst); err != nil {
			return nil, fmt.Errorf("send the search: %w", err)
		}
	}

	var found []Device
	seen := map[string]bool{}
	buf := make([]byte, 4096)
	for {
		n, _, err := conn.ReadFrom(buf)
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				break
			}
			return found, err
		}
		location := ssdpHeader(string(buf[:n]), "location")
		ip := locationHost(location)
		if ip == "" || seen[ip] {
			continue
		}
		seen[ip] = true
		found = append(found, Device{IP: ip, Location: location})
	}
	return found, nil
}

// ssdpHeader returns the value of a header of an SSDP message, matched
// without case.
func ssdpHeader(message, name string) string {
	scan := bufio.NewScanner(strings.NewReader(message))
	for scan.Scan() {
		key, value, ok := strings.Cut(scan.Text(), ":")
		if ok && strings.EqualFold(strings.TrimSpace(key), name) {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// locationHost is the host of a device description URL, and "" when it names
// none.
func locationHost(location string) string {
	if location == "" {
		return ""
	}
	u, err := url.Parse(location)
	if err != nil || u.Host == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(u.Host); err == nil {
		return host
	}
	return u.Host
}
