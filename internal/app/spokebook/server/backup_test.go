package server_test

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/Mik-/spokebook/internal/app/spokebook/auth"
)

// upload posts a multipart form with the file and returns status, final
// path and body.
func (c *client) upload(path string, fields map[string]string, file string) (int, string, string) {
	c.t.Helper()
	return c.uploadFiles(path, fields, "file", map[string]string{"backup.json": file})
}

// uploadFiles posts a multipart form with the files (name to content) in
// field.
func (c *client) uploadFiles(path string, fields map[string]string, field string, files map[string]string) (int, string, string) {
	c.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range fields {
		_ = mw.WriteField(k, v)
	}
	for name, content := range files {
		fw, err := mw.CreateFormFile(field, name)
		if err != nil {
			c.t.Fatal(err)
		}
		_, _ = io.WriteString(fw, content)
	}
	_ = mw.Close()
	req, err := http.NewRequest("POST", c.srv.URL+path, &buf)
	if err != nil {
		c.t.Fatal(err)
	}
	req.Header.Set("Accept-Language", c.lang)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Origin", c.srv.URL)
	res, err := c.srv.Client().Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		c.t.Fatal(err)
	}
	return res.StatusCode, res.Request.URL.Path, string(body)
}

func TestBackupExportImport(t *testing.T) {
	c := newClient(t)
	ctx := context.Background()
	bikePath := c.createBike("Gravel")
	c.uploadFiles(bikePath+"/attachments", nil, "files", map[string]string{"bike.png": pngImage(t)})

	_, _, body := c.do("GET", "/account", nil)
	mustContain(t, body, `href="/account/backup"`, `href="/account/backup/import"`)

	req, _ := http.NewRequest("GET", c.srv.URL+"/account/backup", nil)
	res, err := c.srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	export, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK || !strings.HasPrefix(res.Header.Get("Content-Disposition"), `attachment; filename="spokebook-backup-`) {
		t.Fatalf("export: %d %v", res.StatusCode, res.Header)
	}
	zr, err := zip.NewReader(bytes.NewReader(export), int64(len(export)))
	if err != nil || len(zr.File) != 2 {
		t.Fatalf("export is no archive with data and photo: %v", err)
	}
	data, err := zr.File[0].Open()
	if err != nil {
		t.Fatal(err)
	}
	inner, _ := io.ReadAll(data)
	mustContain(t, string(inner), `"format": "spokebook-backup"`, `"name": "Gravel"`, `"name": "bike.jpg"`, `"cover": true`)

	// After the export, a second bike is added; the import removes it.
	c.createBike("Road")
	_, _, body = c.do("GET", "/account/backup/import", nil)
	mustContain(t, body, `enctype="multipart/form-data"`, `name="confirm"`)

	status, _, body := c.upload("/account/backup/import", nil, string(export))
	if status != http.StatusUnprocessableEntity {
		t.Errorf("without confirmation: %d", status)
	}
	mustContain(t, body, "Bestätige, dass deine Daten ersetzt werden.")
	status, _, body = c.upload("/account/backup/import", map[string]string{"confirm": "1"}, `{"format":"other"}`)
	if status != http.StatusUnprocessableEntity {
		t.Errorf("invalid file: %d", status)
	}
	mustContain(t, body, "keine Spokebook-Sicherung")
	if bikes, _ := c.st.Bikes().ListBikes(ctx, auth.FirstUserID); len(bikes) != 2 {
		t.Errorf("bikes after rejected import: %d", len(bikes))
	}

	status, path, body := c.upload("/account/backup/import", map[string]string{"confirm": "1"}, string(export))
	if status != http.StatusOK || path != "/" {
		t.Fatalf("import: %d %s", status, path)
	}
	mustContain(t, body, "Gravel", `class="card-photo"`)
	if strings.Contains(body, "Road") {
		t.Error("bike from after the export kept")
	}
	// Still logged in.
	if _, path, _ := c.do("GET", "/account", nil); path != "/account" {
		t.Errorf("logged out after import: %s", path)
	}
}
