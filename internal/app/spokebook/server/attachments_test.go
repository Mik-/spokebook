package server_test

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/Mik-/spokebook/internal/app/spokebook/auth"
)

func pngImage(t *testing.T) string {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 40, 30))); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// get fetches path and returns status, headers and body without
// following redirects.
func (c *client) get(path string) (int, http.Header, []byte) {
	c.t.Helper()
	res, err := c.srv.Client().Get(c.srv.URL + path)
	if err != nil {
		c.t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	body, _ := io.ReadAll(res.Body)
	return res.StatusCode, res.Header, body
}

const testPDF = "%PDF-1.4\n%%EOF\n"

func TestAttachments(t *testing.T) {
	c := newClient(t)
	ctx := context.Background()
	bikePath := c.createBike("Gravel")
	_, chainPath, _ := c.do("POST", "/components", url.Values{"type": {"chain"}, "manufacturer": {"KMC"}, "model": {"X11"}})
	_, cassettePath, _ := c.do("POST", "/components", url.Values{"type": {"cassette"}, "manufacturer": {"Shimano"}, "model": {"XT"}})

	_, _, body := c.do("GET", bikePath, nil)
	mustContain(t, body, "Fotos und Belege", `href="`+bikePath+`/attachments/new"`, "Noch keine Fotos oder Belege.")
	_, _, body = c.do("GET", bikePath+"/attachments/new", nil)
	mustContain(t, body, `enctype="multipart/form-data"`, `accept="image/jpeg,`)

	// A file of another kind rejects the whole upload.
	status, _, body := c.uploadFiles(bikePath+"/attachments", nil, "files", map[string]string{"notes.txt": "hello", "a.png": pngImage(t)})
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("text file: %d", status)
	}
	mustContain(t, body, "notes.txt: Möglich sind nur")
	if all, _ := c.st.Attachments().ListAttachments(ctx, auth.FirstUserID); len(all) != 0 {
		t.Fatalf("stored despite error: %d", len(all))
	}

	// The first photo becomes the bike's photo.
	status, path, body := c.uploadFiles(bikePath+"/attachments", nil, "files", map[string]string{"bike.png": pngImage(t)})
	if status != http.StatusOK || path != bikePath {
		t.Fatalf("upload: %d %s", status, path)
	}
	list, _ := c.st.Attachments().ListBikeAttachments(ctx, auth.FirstUserID, strings.TrimPrefix(bikePath, "/bikes/"))
	if len(list) != 1 || !list[0].Cover || list[0].Name != "bike.jpg" {
		t.Fatalf("attachments %+v", list)
	}
	photo := list[0].ID
	mustContain(t, body, `class="bike-photo"><img src="/attachments/`+photo+`"`, "bike.jpg", "Titelbild")
	_, _, body = c.do("GET", "/", nil)
	mustContain(t, body, `src="/attachments/`+photo+`/thumb"`)

	status, h, data := c.get("/attachments/" + photo)
	if status != http.StatusOK || h.Get("Content-Type") != "image/jpeg" || !strings.HasPrefix(h.Get("Content-Disposition"), "inline") || len(data) == 0 {
		t.Fatalf("photo: %d %v", status, h)
	}
	if status, h, _ := c.get("/attachments/" + photo + "/thumb"); status != http.StatusOK || h.Get("Content-Type") != "image/jpeg" {
		t.Fatalf("thumb: %d %v", status, h)
	}

	// A receipt for two components.
	status, _, _ = c.uploadFiles(chainPath+"/attachments", nil, "files", map[string]string{"Rechnung März.pdf": testPDF})
	if status != http.StatusOK {
		t.Fatalf("pdf: %d", status)
	}
	receipts, _ := c.st.Attachments().ListComponentAttachments(ctx, auth.FirstUserID, strings.TrimPrefix(chainPath, "/components/"))
	if len(receipts) != 1 {
		t.Fatalf("receipts %+v", receipts)
	}
	receipt := receipts[0].ID
	// A PDF opens in the browser's viewer under its own policy, or is
	// downloaded on request.
	status, h, data = c.get("/attachments/" + receipt)
	if status != http.StatusOK || h.Get("Content-Type") != "application/pdf" || h.Get("Content-Disposition") != `inline; filename*=utf-8''Rechnung%20M%C3%A4rz.pdf` || string(data) != testPDF {
		t.Fatalf("pdf: %d %v", status, h)
	}
	if csp := h.Get("Content-Security-Policy"); !strings.HasPrefix(csp, "default-src 'none'; object-src 'self';") || h.Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("pdf policy %v", h)
	}
	status, h, _ = c.get("/attachments/" + receipt + "?download")
	if status != http.StatusOK || !strings.HasPrefix(h.Get("Content-Disposition"), "attachment;") {
		t.Fatalf("pdf download: %d %v", status, h)
	}
	if status, _, _ := c.get("/attachments/" + receipt + "/thumb"); status != http.StatusNotFound {
		t.Fatalf("pdf thumb: %d", status)
	}

	_, _, body = c.do("GET", cassettePath+"/attachments/link", nil)
	mustContain(t, body, `value="`+receipt+`"`, "KMC X11", `value="`+photo+`"`)
	status, _, _ = c.do("POST", cassettePath+"/attachments/link", url.Values{})
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("link nothing: %d", status)
	}
	status, path, body = c.do("POST", cassettePath+"/attachments/link", url.Values{"attachment": {receipt}})
	if status != http.StatusOK || path != cassettePath {
		t.Fatalf("link: %d %s", status, path)
	}
	mustContain(t, body, "Rechnung März.pdf")

	// The edit page lists both; removing one keeps the other.
	cassetteID := strings.TrimPrefix(cassettePath, "/components/")
	_, _, body = c.do("GET", "/attachments/"+receipt+"/edit?component="+cassetteID, nil)
	mustContain(t, body, "KMC X11", "Shimano XT", `name="unlink_component"`, `<a href="/components">Komponenten</a>`)
	status, path, _ = c.do("POST", "/attachments/"+receipt+"/unlink?component="+cassetteID, url.Values{"unlink_component": {cassetteID}})
	if status != http.StatusOK || path != cassettePath {
		t.Fatalf("unlink: %d %s", status, path)
	}
	if l, _ := c.st.Attachments().ListAttachmentLinks(ctx, auth.FirstUserID, receipt); len(l) != 1 {
		t.Fatalf("links %+v", l)
	}
	// The last link only goes by deleting.
	chainID := strings.TrimPrefix(chainPath, "/components/")
	_, path, body = c.do("POST", "/attachments/"+receipt+"/unlink?component="+chainID, url.Values{"unlink_component": {chainID}})
	if path != "/attachments/"+receipt+"/delete" {
		t.Fatalf("unlink last: %s", path)
	}
	mustContain(t, body, "„Rechnung März.pdf“ löschen?")

	// Renaming.
	status, _, body = c.do("POST", "/attachments/"+receipt+"?component="+chainID, url.Values{"name": {" "}})
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("empty name: %d", status)
	}
	mustContain(t, body, "Bitte einen Namen angeben.")
	if _, path, _ = c.do("POST", "/attachments/"+receipt+"?component="+chainID, url.Values{"name": {"Kette.pdf"}}); path != chainPath {
		t.Fatalf("rename: %s", path)
	}

	status, path, _ = c.do("POST", "/attachments/"+receipt+"/delete?component="+chainID, url.Values{})
	if status != http.StatusOK || path != chainPath {
		t.Fatalf("delete: %d %s", status, path)
	}
	if status, _, _ := c.get("/attachments/" + receipt); status != http.StatusNotFound {
		t.Fatalf("deleted: %d", status)
	}

	// A second photo can become the bike's photo.
	c.uploadFiles(bikePath+"/attachments", nil, "files", map[string]string{"side.png": pngImage(t)})
	list, _ = c.st.Attachments().ListBikeAttachments(ctx, auth.FirstUserID, strings.TrimPrefix(bikePath, "/bikes/"))
	if len(list) != 2 || list[0].ID != photo {
		t.Fatalf("attachments %+v", list)
	}
	c.do("POST", bikePath+"/cover", url.Values{"attachment": {list[1].ID}})
	if covers, _ := c.st.Attachments().Covers(ctx, auth.FirstUserID); covers[strings.TrimPrefix(bikePath, "/bikes/")] != list[1].ID {
		t.Fatalf("covers %v", covers)
	}

	// The bike can do without a photo; the attachments stay.
	_, _, body = c.do("GET", bikePath, nil)
	mustContain(t, body, "Nicht als Titelbild")
	_, _, body = c.do("POST", bikePath+"/cover", url.Values{"attachment": {""}})
	mustNotContain(t, body, "Nicht als Titelbild", ">Titelbild<")
	if covers, _ := c.st.Attachments().Covers(ctx, auth.FirstUserID); len(covers) != 0 {
		t.Fatalf("covers after clearing %v", covers)
	}
	if list, _ = c.st.Attachments().ListBikeAttachments(ctx, auth.FirstUserID, strings.TrimPrefix(bikePath, "/bikes/")); len(list) != 2 {
		t.Fatalf("attachments after clearing %+v", list)
	}
}

func TestAttachmentsOfOtherOwner(t *testing.T) {
	c := newClient(t)
	if status, _, _ := c.get("/attachments/unknown"); status != http.StatusNotFound {
		t.Fatalf("unknown: %d", status)
	}
	if status, _, _ := c.do("GET", "/bikes/unknown/attachments/new", nil); status != http.StatusNotFound {
		t.Fatalf("unknown bike: %d", status)
	}
}
