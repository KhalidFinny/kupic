package camera

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// panasonicAgent is the client string Panasonic's own traffic uses. The camera
// may only answer the app it knows.
const panasonicAgent = "Panasonic Android/1 DM-CP"

// Diagnose records the camera's raw answers to the access handshake, so the
// reply format can be read off the hardware instead of guessed. It reports
// whether the camera answered at all; a camera that is not there leaves nothing
// worth keeping.
//
// The access reply shape is the one part of this protocol that differs between
// firmware, and guessing it has been wrong every time, so this exists to be
// read rather than inferred.
func Diagnose(ctx context.Context, dev Device) (string, bool) {
	ip := dev.IP
	var b strings.Builder
	fmt.Fprintf(&b, "kupic — the camera's raw access replies\n")
	fmt.Fprintf(&b, "camera %s\n", ip)
	fmt.Fprintf(&b, "run at %s\n", time.Now().Format(time.RFC3339))

	// Does the control interface answer at all? Without this there is nothing
	// to diagnose, and the report would only mislead.
	if _, ok := ask(ctx, &b, ip, "getstate", nil, "", false); !ok {
		return "", false
	}

	b.WriteString("\n-- does it talk? --\n")
	ask(ctx, &b, ip, "getstate", nil, "", true)
	ask(ctx, &b, ip, "accctrl", map[string]string{"type": "req_acc_g"}, "", true)

	// req_acc_g is what hands over the key, and it answers err_param with no
	// parameters, so try the shapes that could satisfy it.
	b.WriteString("\n-- req_acc_g, looking for a key --\n")
	udn := ClientUDN()
	for _, params := range []map[string]string{
		{"type": "req_acc_g"},
		{"type": "req_acc_g", "value": udn},
		{"type": "req_acc_g", "value": udn, "value2": "kupic"},
		{"type": "req_acc_g", "value": "kupic"},
	} {
		ask(ctx, &b, ip, "accctrl", params, "", true)
	}
	b.WriteString("  (with the Panasonic client string)\n")
	ask(ctx, &b, ip, "accctrl", map[string]string{"type": "req_acc_g"}, panasonicAgent, true)

	// The two access requests themselves, so their reply shape is known.
	b.WriteString("\n-- access requests --\n")
	ask(ctx, &b, ip, "accctrl", map[string]string{
		"type": "req_acc", "value": udn, "value2": "kupic",
	}, "", true)
	ask(ctx, &b, ip, "accctrl", map[string]string{
		"type": "req_acc_e", "value": "00", "value2": "00",
	}, "", true)

	// Everything after access: a camera can accept the session and still
	// refuse to be named or to list its card, and that is a different fault.
	b.WriteString("\n-- after access --\n")
	ask(ctx, &b, ip, "setsetting", map[string]string{
		"type": "device_name", "value": "kupic",
	}, "", true)
	ask(ctx, &b, ip, "camcmd", map[string]string{"value": "playmode"}, "", true)
	ask(ctx, &b, ip, "getstate", nil, "", true)

	// The card listing itself, which is a SOAP POST rather than cam.cgi. The
	// camera can answer a full page of items and still withhold every file
	// URL, so the report asks each shape that could differ, at the size the
	// app asks for, and counts what came back with a <res>.
	b.WriteString("\n-- card listing --\n")
	for i, shape := range listingShapes() {
		status, body, err := askBrowse(ctx, ip, controlPath, shape, pageSize)
		if err != nil {
			fmt.Fprintf(&b, "  %-44s %v\n", "browse, "+shape.name, err)
			continue
		}
		items, withRes := countDIDL(body)
		fmt.Fprintf(&b, "  %-44s %s, %d items, %d with res\n",
			"browse, "+shape.name, status, items, withRes)
		if i == 0 {
			fmt.Fprintf(&b, "      %s\n", oneLine(body))
		}
	}
	// The description names a control path of its own. The app used to take
	// it, so record it and ask it too: if the card is served there and not on
	// the known path, that is the whole fault.
	if path := controlURL(ctx, dev.Location); path != "" {
		fmt.Fprintf(&b, "  %-44s %s\n", "described control path", path)
		if path != controlPath {
			status, body, err := askBrowse(ctx, ip, path, listingShapes()[0], pageSize)
			if err != nil {
				fmt.Fprintf(&b, "  %-44s %v\n", "browse, described path", err)
			} else {
				items, withRes := countDIDL(body)
				fmt.Fprintf(&b, "  %-44s %s, %d items, %d with res\n",
					"browse, described path", status, items, withRes)
			}
		}
	}
	return b.String(), true
}

// controlURL reads the ContentDirectory control path out of the camera's
// description, which is where the app used to take it from.
func controlURL(ctx context.Context, location string) string {
	if location == "" {
		return ""
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, location, nil)
	if err != nil {
		return ""
	}
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))

	var doc struct {
		Device struct {
			Services []struct {
				Type       string `xml:"serviceType"`
				ControlURL string `xml:"controlURL"`
			} `xml:"serviceList>service"`
		} `xml:"device"`
	}
	if err := xml.Unmarshal(body, &doc); err != nil {
		return ""
	}
	for _, s := range doc.Device.Services {
		if strings.Contains(s.Type, "ContentDirectory") {
			return s.ControlURL
		}
	}
	return ""
}

// askBrowse asks the content directory for the first page of the card with one
// request shape, so its answer is recorded even when it fails.
func askBrowse(ctx context.Context, ip, path string, shape listingShape, count int) (string, string, error) {
	envelope := fmt.Sprintf(browseEnvelope, shape.filter, 0, count, shape.xfilter)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"http://"+ip+":"+upnpPort+path, strings.NewReader(envelope))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", `text/xml; charset="utf-8"`)
	req.Header.Set("SOAPACTION", `"urn:schemas-upnp-org:service:ContentDirectory:1#Browse"`)

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	return resp.Status, string(raw), nil
}

// countDIDL reports how many items an answer lists, and how many of them carry
// a file URL.
func countDIDL(body string) (items, withRes int) {
	var env struct {
		Body struct {
			Browse struct {
				Result string `xml:"Result"`
			} `xml:"BrowseResponse"`
		} `xml:"Body"`
	}
	if err := xml.Unmarshal([]byte(body), &env); err != nil {
		return 0, 0
	}
	didl := decodeDIDL(env.Body.Browse.Result)
	return strings.Count(didl, "<item "), strings.Count(didl, "<res ")
}

// ask sends one control-interface request and records its status, content type
// and whole body when write is set. It reports whether the camera answered.
func ask(ctx context.Context, b *strings.Builder, ip, kind string, params map[string]string, agent string, write bool) (string, bool) {
	target := "http://" + ip + "/cam.cgi?" + query(kind, params)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", false
	}
	if agent != "" {
		req.Header.Set("User-Agent", agent)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		if write {
			fmt.Fprintf(b, "  %-44s %v\n", describeRequest(kind, params), err)
		}
		return "", false
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	text := string(body)
	if write {
		fmt.Fprintf(b, "  %-44s %s\n      %s\n",
			describeRequest(kind, params), resp.Status, oneLine(text))
	}
	return text, true
}

// query builds the control-interface query string for one request.
func query(kind string, params map[string]string) string {
	values := url.Values{}
	if kind == "getstate" {
		values.Set("mode", "getstate")
		return values.Encode()
	}
	values.Set("mode", kind)
	for k, v := range params {
		values.Set(k, v)
	}
	return values.Encode()
}

// describeRequest names a control request the way the report lists it.
func describeRequest(kind string, params map[string]string) string {
	if kind == "getstate" {
		return "getstate"
	}
	if len(params) == 0 {
		return "accctrl"
	}
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+params[k])
	}
	return strings.Join(parts, "&")
}

// oneLine collapses a body so it reads as one line in the report.
func oneLine(body string) string {
	return strings.Join(strings.Fields(body), " ")
}
