package camera

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// The camera's control interface answers on port 80, its UPnP server — the
// device description and the content directory — on 60606, and the files it
// offers on 50001.
const (
	camCgiPath  = "/cam.cgi"
	camPort     = "80"
	upnpPort    = "60606"
	filePort    = "50001"
	controlPath = "/Server0/CDS_control"
)

// How long to keep asking the camera for a session, and how often. A camera
// that has just been woken is busy for a few seconds.
const (
	sessionWait  = 20 * time.Second
	sessionRetry = 500 * time.Millisecond
)

// Client is a session with a Lumix camera over Wi-Fi.
type Client struct {
	ip         string
	clientName string
	udn        string
	model      string
	http       *http.Client
}

// Connect opens a session with the camera at dev, presenting name to it. The
// camera must be in Wi-Fi connection standby.
func Connect(ctx context.Context, dev Device, name string) (*Client, error) {
	c := &Client{ip: dev.IP, clientName: name,
		http: &http.Client{Timeout: 20 * time.Second}}
	if err := c.describe(ctx, dev.Location); err != nil {
		return nil, err
	}
	if err := c.pair(ctx); err != nil {
		return nil, err
	}
	if err := c.present(ctx); err != nil {
		return nil, err
	}
	return c, nil
}

// Name is the camera's model, as it reports it.
func (c *Client) Name() string { return c.model }

// IP is the camera's address.
func (c *Client) IP() string { return c.ip }

// describe reads the camera's UPnP description: the UDN names the session and
// the model names the camera.
func (c *Client) describe(ctx context.Context, location string) error {
	body, err := c.get(ctx, location)
	if err != nil {
		return err
	}
	var doc struct {
		Device struct {
			FriendlyName string `xml:"friendlyName"`
			ModelName    string `xml:"modelName"`
			UDN          string `xml:"UDN"`
		} `xml:"device"`
	}
	if err := xml.Unmarshal(body, &doc); err != nil {
		return fmt.Errorf("read the camera description: %w", err)
	}
	c.udn = strings.TrimPrefix(doc.Device.UDN, "uuid:")
	c.model = doc.Device.ModelName
	if c.model == "" {
		c.model = doc.Device.FriendlyName
	}
	if c.udn == "" {
		return fmt.Errorf("the camera at %s did not describe itself", c.ip)
	}
	return nil
}

// pair asks the camera to accept this client. The plain request is what this
// firmware accepts; the encrypted form, whose key comes from req_acc_g, is the
// fallback for a camera that refuses it.
//
// It retries until the camera answers "ok". A camera that is still busy with
// an earlier session answers "ok_under_research_no_msg", which is not an
// acceptance: proceeding on it leaves every later request refused. liblumix
// waits the same way, half a second at a time.
func (c *Client) pair(ctx context.Context) error {
	key, _ := c.sessionKey(ctx)
	deadline := time.Now().Add(sessionWait)
	for {
		err := c.requestAccess(ctx, "req_acc", 0)
		if err == nil {
			return nil
		}
		if key != 0 {
			if err = c.requestAccess(ctx, "req_acc_e", key); err == nil {
				return nil
			}
		}
		if time.Now().After(deadline) {
			return err
		}
		select {
		case <-ctx.Done():
			return err
		case <-time.After(sessionRetry):
		}
	}
}

// requestAccess sends one access request and checks that the camera accepted
// it. req_acc_e takes the two values encoded; req_acc takes them as they are.
func (c *Client) requestAccess(ctx context.Context, kind string, key uint32) error {
	udn, name := ClientUDN(), c.clientName
	if kind == "req_acc_e" {
		udn, name = encodeSession(udn, key), encodeSession(name, key)
	}
	body, err := c.camcgi(ctx, url.Values{
		"mode":   {"accctrl"},
		"type":   {kind},
		"value":  {udn},
		"value2": {name},
	})
	if err != nil {
		return err
	}
	// Older firmware answers "<status>,<session id>" in plain text; newer
	// answers <camrply><result>…</result></camrply>. Only "ok" is an
	// acceptance: the camera answers other "ok_…" words while it is busy, and
	// they are not a session.
	if answer, _, _ := strings.Cut(strings.TrimSpace(string(body)), ","); answer == "ok" || resultOf(body) == "ok" {
		return nil
	}
	return fmt.Errorf("the camera refused the connection: %s", resultOf(body))
}

// resultOf reads the camera's own word for what happened: the <result> element
// when the answer is XML, else the first line. It keeps an error short enough
// to read in the window.
func resultOf(body []byte) string {
	text := string(body)
	if _, rest, ok := strings.Cut(text, "<result>"); ok {
		if result, _, ok := strings.Cut(rest, "</result>"); ok {
			if result = strings.TrimSpace(result); result != "" {
				return result
			}
		}
	}
	if line := firstLine(body); line != "" {
		return line
	}
	return "no reason given"
}

// present names this client on the camera. It is best effort: the session is
// what grants access, and a camera that will not take the name is still
// readable, so a refusal here is not worth failing the connection over.
func (c *Client) present(ctx context.Context) error {
	_, err := c.camcgi(ctx, url.Values{
		"mode":  {"setsetting"},
		"type":  {"device_name"},
		"value": {c.clientName},
	})
	return err
}

// camcgi calls the camera's control interface, which answers on port 80.
func (c *Client) camcgi(ctx context.Context, params url.Values) ([]byte, error) {
	return c.get(ctx, "http://"+c.ip+":"+camPort+camCgiPath+"?"+params.Encode())
}

// get fetches an absolute URL from the camera.
func (c *Client) get(ctx context.Context, target string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("reach the camera at %s: %w", c.ip, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the camera answered %s for %s", resp.Status, target)
	}
	return body, nil
}

// Open starts downloading a file from the camera, for a caller that streams
// it to disk. The caller closes it.
func (c *Client) Open(ctx context.Context, fileURL string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.absolute(fileURL, filePort), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download from the camera: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("the camera answered %s", resp.Status)
	}
	return resp.Body, nil
}

// absolute resolves a URL the camera gave, which may be relative, against the
// port that serves it. The control interface and the files are on different
// ports, so the caller says which.
func (c *Client) absolute(target, port string) string {
	if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
		return target
	}
	return "http://" + c.ip + ":" + port + "/" + strings.TrimPrefix(target, "/")
}

// firstLine trims a response to something an error can hold.
func firstLine(body []byte) string {
	line, _, _ := strings.Cut(strings.TrimSpace(string(body)), "\n")
	if len(line) > 120 {
		line = line[:120] + "…"
	}
	return line
}
