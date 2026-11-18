package camera

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"
)

// File is one representation of a photo on the card, as the camera offers
// it: the JPEG, the RAW, or a thumbnail.
type File struct {
	URL  string
	MIME string
	Size int64
	Name string
	// Profile is the DLNA profile the camera gave the file, when it gave
	// one: JPEG_TN is the thumbnail, JPEG_MED a medium copy, JPEG_LRG the
	// full-size JPEG.
	Profile string
}

// Photo is one item on the card.
type Photo struct {
	ID    string
	Title string
	Date  time.Time
	Files []File
}

// JPEG returns the photo's JPEG file, the largest that is not a thumbnail.
func (p Photo) JPEG() (File, bool) {
	return p.pick(func(f File) bool { return isJPEG(f) && !isThumb(f) })
}

// RAW returns the photo's RAW file, if it was shot as one.
func (p Photo) RAW() (File, bool) { return p.pick(isRAW) }

// Thumb returns the file to show as a thumbnail: the camera's own thumbnail
// where it offers one, else the thumbnail a Lumix keeps beside the photo
// (DO1000001.JPG's is DT1000001.JPG), else the smallest image the source has —
// which on a mounted card is a local read, and off the camera is a fallback
// worth avoiding.
func (p Photo) Thumb() (File, bool) {
	for _, f := range p.Files {
		if isThumb(f) {
			return f, true
		}
	}
	if jpeg, ok := p.JPEG(); ok {
		if thumb, ok := thumbFor(jpeg); ok {
			return thumb, true
		}
	}
	var best File
	found := false
	for _, f := range p.Files {
		if !isJPEG(f) && !isRAW(f) {
			continue
		}
		if !found || f.Size < best.Size {
			best, found = f, true
		}
	}
	return best, found
}

// thumbFor builds the thumbnail that sits beside a Lumix photo. The camera's
// own names are the only way to know it: DO1000001.JPG's is DT1000001.JPG.
func thumbFor(f File) (File, bool) {
	name := path.Base(f.URL)
	if len(name) < 3 || !strings.HasPrefix(strings.ToUpper(name), "DO") {
		return File{}, false
	}
	thumb := "DT" + name[2:]
	return File{
		URL:     strings.TrimSuffix(f.URL, name) + thumb,
		Name:    thumb,
		MIME:    "image/jpeg",
		Profile: "JPEG_TN",
	}, true
}

// pick returns the largest of the files that match. The camera does not
// always give a size, and then the first match wins.
func (p Photo) pick(match func(File) bool) (File, bool) {
	var best File
	found := false
	for _, f := range p.Files {
		if match(f) && (!found || f.Size > best.Size) {
			best, found = f, true
		}
	}
	return best, found
}

// pageSize is how many items one Browse asks for. The camera answers a
// bounded page, so the card is read in pages of this size.
const pageSize = 64

// maxPhotos caps a listing, so that a camera that never reports its total
// cannot be read forever.
const maxPhotos = 50000

// listingShape is one request shape for the card listing. The camera is strict
// about both filter elements: it answers 500 without pana:X_Filter, and which
// filters it accepts decides whether the items come back with their file URLs.
type listingShape struct {
	name    string
	filter  string
	xfilter string
}

// listingShapes are the shapes worth asking, most likely first. The camera
// answers the first with a full page but no <res> elements, so the others ask
// for the resources by name instead of by wildcard.
func listingShapes() []listingShape {
	return []listingShape{
		{"app's", "*", ""},
		{"X_Filter=image", "*", "image"},
		{"Filter=res", "res", ""},
		{"Filter=res,res@protocolInfo", "res,res@protocolInfo,res@size", ""},
	}
}

// List reads the card's photos, in pages, newest first.
func (c *Client) List(ctx context.Context) ([]Photo, error) {
	// The card is only served in playback mode: in recording mode the
	// content directory answers 500 for it.
	c.playback(ctx)

	var (
		firstErr   error
		firstShape listingShape
		first      []didlItem
		total      int
	)
	for _, shape := range listingShapes() {
		items, matches, err := c.browse(ctx, shape, 0, pageSize)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		photos := photosOf(items)
		if len(photos) == 0 {
			// The camera lists the card but not where the files are; a
			// later shape may still name them, so keep this one in hand.
			if first == nil {
				firstShape, first, total = shape, items, matches
			}
			continue
		}
		for start := pageSize; start < matches && len(photos) < maxPhotos; start += pageSize {
			more, _, err := c.browse(ctx, shape, start, pageSize)
			if err != nil {
				return photos, err
			}
			if len(more) == 0 {
				break
			}
			photos = append(photos, photosOf(more)...)
		}
		return photos, nil
	}

	if len(first) > 0 {
		return c.named(ctx, firstShape, first, total)
	}
	if firstErr != nil {
		return nil, firstErr
	}
	return nil, nil
}

// playback puts the camera into playback mode. The card is only served in that
// mode, and the camera needs a moment to switch, which is why the reference
// clients wait here too.
//
// It is best effort: if the camera refuses, the listing's own error says more
// than this would, so it is not worth failing the read over.
func (c *Client) playback(ctx context.Context) {
	if _, err := c.camcgi(ctx, url.Values{"mode": {"camcmd"}, "value": {"playmode"}}); err != nil {
		return
	}
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
	}
}

// browse reads one page of the card's content directory.
func (c *Client) browse(ctx context.Context, shape listingShape, start, count int) ([]didlItem, int, error) {
	body := fmt.Sprintf(browseEnvelope, shape.filter, start, count, shape.xfilter)
	// Both reference clients hardcode this path on the UPnP port, and it is
	// the one the camera answered.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.absolute(controlPath, upnpPort), strings.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", `text/xml; charset="utf-8"`)
	req.Header.Set("SOAPACTION", `"urn:schemas-upnp-org:service:ContentDirectory:1#Browse"`)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("read the camera's card: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, 0, err
	}
	if resp.StatusCode != http.StatusOK {
		// The camera explains itself in the body, and without it a 500 is
		// impossible to tell from a bad URL or a bad session.
		return nil, 0, fmt.Errorf("the camera answered %s listing the card: %s",
			resp.Status, refusal(raw))
	}

	var env struct {
		Body struct {
			Browse struct {
				Result         string `xml:"Result"`
				NumberReturned int    `xml:"NumberReturned"`
				TotalMatches   int    `xml:"TotalMatches"`
			} `xml:"BrowseResponse"`
		} `xml:"Body"`
	}
	if err := xml.Unmarshal(raw, &env); err != nil {
		return nil, 0, fmt.Errorf("read the camera's answer: %w", err)
	}
	items, err := parseDIDL(env.Body.Browse.Result)
	if err != nil {
		return nil, 0, err
	}
	return items, env.Body.Browse.TotalMatches, nil
}

// named builds the card's photos from the item names alone, for a camera that
// lists its card without saying where the files are. A Lumix serves the photo
// as DO<n>.JPG and its thumbnail as DT<n>.JPG; n is the item id, but whether
// the leading media-type digit belongs to the file name is not certain across
// firmware, so the first item decides it.
func (c *Client) named(ctx context.Context, shape listingShape, first []didlItem, total int) ([]Photo, error) {
	base := "http://" + c.ip + ":" + filePort + "/"
	// A video, or a RAW-only shot, has no DO<n>.JPG, so a few items decide
	// the rule rather than the first one alone.
	rule := -1
	for i := range fileNumbers(first[0].ID) {
		for _, item := range first[:min(len(first), 3)] {
			if c.serves(ctx, base+"DO"+fileNumber(item.ID, i)+".JPG") {
				rule = i
				break
			}
		}
		if rule >= 0 {
			break
		}
	}
	if rule < 0 {
		return nil, fmt.Errorf("the camera listed %d photos but gave no file URLs", total)
	}

	items := first
	for start := pageSize; start < total && len(items) < maxPhotos; start += pageSize {
		more, _, err := c.browse(ctx, shape, start, pageSize)
		if err != nil || len(more) == 0 {
			break
		}
		items = append(items, more...)
	}

	photos := make([]Photo, 0, len(items))
	for _, item := range items {
		n := fileNumber(item.ID, rule)
		photos = append(photos, Photo{
			ID:    item.ID,
			Title: item.Title,
			Date:  parseDate(item.Date),
			Files: []File{
				{URL: base + "DO" + n + ".JPG", Name: "DO" + n + ".JPG",
					MIME: "image/jpeg", Profile: "JPEG_LRG"},
				{URL: base + "DT" + n + ".JPG", Name: "DT" + n + ".JPG",
					MIME: "image/jpeg", Profile: "JPEG_TN"},
			},
		})
	}
	return photos, nil
}

// serves reports whether the camera hands over one file. It asks for a single
// byte, so probing a name does not pull a whole photo across the network.
func (c *Client) serves(ctx context.Context, target string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return false
	}
	req.Header.Set("Range", "bytes=0-0")
	resp, err := c.http.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<10))
	return resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusPartialContent
}

// fileNumbers are the ways a Lumix item id can map to the file name: the id
// without its leading media-type digit, and the id itself.
func fileNumbers(id string) []string {
	if len(id) > 1 {
		return []string{id[1:], id}
	}
	return []string{id}
}

// fileNumber picks the rule-th form of an id, for an id too short to have it.
func fileNumber(id string, rule int) string {
	numbers := fileNumbers(id)
	if rule >= len(numbers) {
		rule = len(numbers) - 1
	}
	return numbers[rule]
}

// didlItem is one item of a content directory answer.
type didlItem struct {
	ID    string    `xml:"id,attr"`
	Title string    `xml:"title"`
	Date  string    `xml:"date"`
	Res   []didlRes `xml:"res"`
}

// didlRes is one file of an item.
type didlRes struct {
	Protocol string `xml:"protocolInfo,attr"`
	Size     int64  `xml:"size,attr"`
	URL      string `xml:",chardata"`
}

// parseDIDL turns the content directory's answer into its items.
func parseDIDL(result string) ([]didlItem, error) {
	var didl struct {
		Items []didlItem `xml:"item"`
	}
	if err := xml.Unmarshal([]byte(decodeDIDL(result)), &didl); err != nil {
		return nil, fmt.Errorf("read the card's contents: %w", err)
	}
	return didl.Items, nil
}

// photosOf turns the items that name their files into photos, dropping the
// ones that do not.
func photosOf(items []didlItem) []Photo {
	var photos []Photo
	for _, item := range items {
		p := Photo{ID: item.ID, Title: item.Title, Date: parseDate(item.Date)}
		for _, res := range item.Res {
			f := File{
				URL:     strings.TrimSpace(res.URL),
				Size:    res.Size,
				MIME:    res.mime(),
				Profile: res.profile(),
			}
			if f.URL == "" {
				continue
			}
			f.Name = path.Base(f.URL)
			p.Files = append(p.Files, f)
		}
		if _, ok := p.Thumb(); ok {
			photos = append(photos, p)
		}
	}
	sort.SliceStable(photos, func(i, j int) bool { return photos[i].Date.After(photos[j].Date) })
	return photos
}

// refusal reads the reason out of a failed answer. A UPnP fault carries an
// error code and description, which says far more than the envelope around it.
func refusal(body []byte) string {
	var fault struct {
		Body struct {
			Fault struct {
				Detail struct {
					Error struct {
						Code        string `xml:"errorCode"`
						Description string `xml:"errorDescription"`
					} `xml:"UPnPError"`
				} `xml:"detail"`
			} `xml:"Fault"`
		} `xml:"Body"`
	}
	if err := xml.Unmarshal(body, &fault); err == nil {
		e := fault.Body.Fault.Detail.Error
		switch {
		case e.Code != "" && e.Description != "":
			return "UPnP error " + e.Code + ": " + e.Description
		case e.Code != "":
			return "UPnP error " + e.Code
		}
	}
	if line := firstLine(body); line != "" {
		return line
	}
	return "no reason given"
}

// decodeDIDL undoes the percent-encoding the camera wraps the result in.
func decodeDIDL(result string) string {
	if !strings.Contains(strings.ToLower(result), "%3c") {
		return result
	}
	if decoded, err := url.QueryUnescape(result); err == nil {
		return decoded
	}
	return result
}

// parseDate reads the date the camera gives an item, which is not always
// present.
func parseDate(value string) time.Time {
	for _, layout := range []string{"2006-01-02T15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, value); err == nil {
			return t
		}
	}
	return time.Time{}
}

func isJPEG(f File) bool {
	name := strings.ToUpper(f.Name)
	return strings.HasSuffix(name, ".JPG") || strings.HasSuffix(name, ".JPEG") ||
		strings.Contains(f.MIME, "jpeg")
}

// isThumb reports whether a file is a photo's thumbnail. The camera says so in
// the DLNA profile, or names the file for it: a Lumix thumbnail is DT1000001.JPG
// where the full JPEG is DO1000001.JPG.
func isThumb(f File) bool {
	if strings.Contains(f.Profile, "TN") {
		return true
	}
	name := strings.ToUpper(f.Name)
	return strings.HasPrefix(name, "DT") || strings.Contains(name, "_T.")
}

func isRAW(f File) bool {
	switch strings.ToUpper(path.Ext(f.Name)) {
	case ".RW2", ".RAW", ".DNG":
		return true
	}
	return strings.Contains(f.MIME, "raw")
}

// mime is the media type of a res, which the camera puts in protocolInfo
// as http-get:*:<mime>:<extra>.
func (r didlRes) mime() string {
	if parts := strings.Split(r.Protocol, ":"); len(parts) >= 3 {
		return parts[2]
	}
	return ""
}

// profile is the DLNA profile of a res, which the camera puts in protocolInfo
// as DLNA.ORG_PN=<profile>. It is how a thumbnail is told from the full file
// when the camera gives no size.
func (r didlRes) profile() string {
	for _, part := range strings.Split(r.Protocol, ";") {
		if profile, ok := strings.CutPrefix(part, "DLNA.ORG_PN="); ok {
			return profile
		}
	}
	return ""
}

// browseEnvelope is the SOAP request that lists the card's contents, the shape
// LUMIX Sync and liblumix send. The camera requires pana:X_Filter — without it
// it answers 500 — and the values are the two filters, the first item, and how
// many to return.
const browseEnvelope = `<?xml version="1.0" encoding="utf-8"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
  <s:Body>
    <u:Browse xmlns:u="urn:schemas-upnp-org:service:ContentDirectory:1" xmlns:pana="urn:schemas-panasonic-com:pana">
      <ObjectID>0</ObjectID>
      <BrowseFlag>BrowseDirectChildren</BrowseFlag>
      <Filter>%s</Filter>
      <StartingIndex>%d</StartingIndex>
      <RequestedCount>%d</RequestedCount>
      <SortCriteria></SortCriteria>
      <pana:X_FromCP>LumixLink2.0</pana:X_FromCP>
      <pana:X_Filter>%s</pana:X_Filter>
    </u:Browse>
  </s:Body>
</s:Envelope>`
