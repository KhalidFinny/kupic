package camera

import (
	"net/url"
	"testing"
)

// didlResult is a content directory answer shaped as a Lumix camera sends
// one: percent-encoded DIDL-Lite with a JPEG, a RAW and a thumbnail for
// each item, newest first.
func didlResult() string {
	raw := `<?xml version="1.0"?>
<DIDL-Lite xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/">
<item id="0$1$0$0" parentID="0$1$0" restricted="1">
  <dc:title>P1004200</dc:title>
  <dc:date>2026-10-08T14:32:10</dc:date>
  <res protocolInfo="http-get:*:image/jpeg:DLNA.ORG_PN=JPEG_LRG" size="6420992">http://192.168.54.1:60606/Server0/DLNA/P1004200.JPG</res>
  <res protocolInfo="http-get:*:image/x-panasonic-rw2:*" size="27033152">http://192.168.54.1:60606/Server0/DLNA/P1004200.RW2</res>
  <res protocolInfo="http-get:*:image/jpeg:DLNA.ORG_PN=JPEG_TN" size="28672">http://192.168.54.1:60606/Server0/DLNA/P1004200_t.JPG</res>
</item>
<item id="0$1$0$1" parentID="0$1$0" restricted="1">
  <dc:title>P1004199</dc:title>
  <dc:date>2026-10-08T14:30:02</dc:date>
  <res protocolInfo="http-get:*:image/jpeg:DLNA.ORG_PN=JPEG_LRG" size="6112256">http://192.168.54.1:60606/Server0/DLNA/P1004199.JPG</res>
  <res protocolInfo="http-get:*:image/jpeg:DLNA.ORG_PN=JPEG_TN" size="27136">http://192.168.54.1:60606/Server0/DLNA/P1004199_t.JPG</res>
</item>
</DIDL-Lite>`
	return url.QueryEscape(raw)
}

// photosFrom parses a content directory answer into the photos a listing
// would show.
func photosFrom(t *testing.T, result string) []Photo {
	t.Helper()
	items, err := parseDIDL(result)
	if err != nil {
		t.Fatalf("parseDIDL: %v", err)
	}
	return photosOf(items)
}

func TestParseDIDL(t *testing.T) {
	photos := photosFrom(t, didlResult())
	if len(photos) != 2 {
		t.Fatalf("got %d photos, want 2", len(photos))
	}
	first := photos[0]
	if first.Title != "P1004200" {
		t.Errorf("title %q, want P1004200", first.Title)
	}
	if first.Date.Year() != 2026 {
		t.Errorf("date %v, want 2026", first.Date)
	}
	if len(first.Files) != 3 {
		t.Fatalf("got %d files, want 3", len(first.Files))
	}

	jpeg, ok := first.JPEG()
	if !ok || jpeg.Name != "P1004200.JPG" {
		t.Errorf("JPEG = %+v, ok %v", jpeg, ok)
	}
	raw, ok := first.RAW()
	if !ok || raw.Name != "P1004200.RW2" {
		t.Errorf("RAW = %+v, ok %v", raw, ok)
	}
	thumb, ok := first.Thumb()
	if !ok || thumb.Name != "P1004200_t.JPG" {
		t.Errorf("Thumb = %+v, ok %v", thumb, ok)
	}

	if _, ok := photos[1].RAW(); ok {
		t.Error("the second photo has no RAW, but one was reported")
	}
	if jpeg, ok := photos[1].JPEG(); !ok || jpeg.Size != 6112256 {
		t.Errorf("second JPEG = %+v, ok %v; want size 6112256", jpeg, ok)
	}
}

// lumixResult is a content directory answer shaped the way a current Lumix
// sends one: no size on any res, the full file as DO…, the thumbnail as DT…
// and a medium copy as DL…, each carrying its DLNA profile.
func lumixResult() string {
	return `<DIDL-Lite xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/">
<item id="1030822" parentID="0" restricted="0">
  <dc:title>103-0822</dc:title>
  <dc:date>2026-10-08T14:32:10</dc:date>
  <upnp:class name="imageItem">object.item.imageItem</upnp:class>
  <res protocolInfo="http-get:*:image/jpeg:DLNA.ORG_PN=JPEG_LRG;DLNA.ORG_OP=01">http://192.168.54.1:50001/DO1030822.JPG</res>
  <res protocolInfo="http-get:*:image/jpeg:DLNA.ORG_PN=JPEG_TN;DLNA.ORG_OP=01">http://192.168.54.1:50001/DT1030822.JPG</res>
  <res protocolInfo="http-get:*:image/jpeg:DLNA.ORG_PN=JPEG_MED;DLNA.ORG_OP=01">http://192.168.54.1:50001/DL1030822.JPG</res>
  <res protocolInfo="http-get:*:application/octet-stream;PANASONIC.COM_PN=CAM_RAW">http://192.168.54.1:50001/DO1030822.RW2</res>
</item>
</DIDL-Lite>`
}

func TestPicksByProfileWithoutSizes(t *testing.T) {
	photos := photosFrom(t, lumixResult())
	if len(photos) != 1 {
		t.Fatalf("got %d photos, want 1", len(photos))
	}
	p := photos[0]

	// With no size on any res, choosing by size hands back whichever file
	// came first — which is the full JPEG, so the grid would load the card
	// at full size.
	thumb, ok := p.Thumb()
	if !ok || thumb.Name != "DT1030822.JPG" {
		t.Errorf("Thumb = %+v, ok %v; want DT1030822.JPG", thumb, ok)
	}
	jpeg, ok := p.JPEG()
	if !ok || jpeg.Name != "DO1030822.JPG" {
		t.Errorf("JPEG = %+v, ok %v; want DO1030822.JPG", jpeg, ok)
	}
	raw, ok := p.RAW()
	if !ok || raw.Name != "DO1030822.RW2" {
		t.Errorf("RAW = %+v, ok %v; want DO1030822.RW2", raw, ok)
	}
}

func TestAbsoluteUsesThePortThatServesIt(t *testing.T) {
	c := &Client{ip: "192.168.54.1"}
	cases := []struct{ target, port, want string }{
		// A file the camera offered without a host: files live on 50001.
		{"/DT1030822.JPG", filePort, "http://192.168.54.1:50001/DT1030822.JPG"},
		// An absolute URL is left as the camera gave it.
		{"http://192.168.54.1:50001/DO1.JPG", filePort, "http://192.168.54.1:50001/DO1.JPG"},
		// The content directory is on the UPnP port, not the file one.
		{"/Server0/CDS_control", upnpPort, "http://192.168.54.1:60606/Server0/CDS_control"},
	}
	for _, tc := range cases {
		if got := c.absolute(tc.target, tc.port); got != tc.want {
			t.Errorf("absolute(%q, %q) = %q, want %q", tc.target, tc.port, got, tc.want)
		}
	}
}

func TestParseDIDLPlain(t *testing.T) {
	// A camera that answers with the DIDL-Lite not percent-encoded.
	photos := photosFrom(t, `<DIDL-Lite><item id="a"><dc:title>X</dc:title>`+
		`<res protocolInfo="http-get:*:image/jpeg:*" size="10">http://cam/X.JPG</res></item></DIDL-Lite>`)
	if len(photos) != 1 || photos[0].Title != "X" {
		t.Fatalf("got %+v", photos)
	}
}

func TestParseDIDLSkipsNonImages(t *testing.T) {
	photos := photosFrom(t, `<DIDL-Lite><item id="v"><dc:title>Clip</dc:title>`+
		`<res protocolInfo="http-get:*:video/mp4:*" size="10">http://cam/clip.MP4</res></item></DIDL-Lite>`)
	if len(photos) != 0 {
		t.Fatalf("got %d photos, want none", len(photos))
	}
}

func TestLocationHost(t *testing.T) {
	cases := map[string]string{
		"http://192.168.54.1:60606/A0C9/Server0/ddd": "192.168.54.1",
		"http://10.0.0.5:60606/x":                    "10.0.0.5",
		"":                                           "",
	}
	for in, want := range cases {
		if got := locationHost(in); got != want {
			t.Errorf("locationHost(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSSDPHeader(t *testing.T) {
	message := "HTTP/1.1 200 OK\r\nST: urn:schemas-upnp-org:service:ContentDirectory:1\r\nLOCATION: http://192.168.54.1:60606/A0C9/Server0/ddd\r\n\r\n"
	if got := ssdpHeader(message, "location"); got != "http://192.168.54.1:60606/A0C9/Server0/ddd" {
		t.Errorf("location = %q", got)
	}
	if got := ssdpHeader(message, "ST"); got != "urn:schemas-upnp-org:service:ContentDirectory:1" {
		t.Errorf("st = %q", got)
	}
}
