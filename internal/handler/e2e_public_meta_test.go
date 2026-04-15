//go:build e2e
// +build e2e

package handler_test

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMeta_AdTypes exercises GET /api/v1/ad-types. The handler returns a
// JSON array describing every entry in campaign.AdTypeConfig. We assert 200
// plus the presence of the four canonical ad type strings.
//
// HandleAdTypes does not read the advertiser from context, so execPublic
// is sufficient.
func TestMeta_AdTypes(t *testing.T) {
	d := mustDeps(t)

	req := authedReq(t, http.MethodGet, "/api/v1/ad-types", nil, "")
	w := execPublic(t, d, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /ad-types: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, want := range []string{"banner", "native", "interstitial", "splash"} {
		if !contains(body, want) {
			t.Fatalf("GET /ad-types: expected body to contain %q, got %s", want, body)
		}
	}
}

// TestMeta_BillingModels exercises GET /api/v1/billing-models. The handler
// returns a JSON array describing entries in campaign.BillingModelConfig.
// We assert 200 plus the presence of "cpm".
func TestMeta_BillingModels(t *testing.T) {
	d := mustDeps(t)

	req := authedReq(t, http.MethodGet, "/api/v1/billing-models", nil, "")
	w := execPublic(t, d, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /billing-models: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !contains(w.Body.String(), "cpm") {
		t.Fatalf("GET /billing-models: expected body to contain \"cpm\", got %s", w.Body.String())
	}
}

// TestMeta_AuditLog exercises GET /api/v1/audit-log. HandleMyAuditLog reads
// the advertiser id from the request context, so this test must drive the
// request through execAuthed (the real auth middleware) to populate it.
//
// A fresh advertiser has no audit entries, so the response is an empty array
// — we only assert the 200 status and that the body decodes as a JSON array.
func TestMeta_AuditLog(t *testing.T) {
	d := mustDeps(t)
	_, apiKey := newAdvertiser(t, d)

	req := authedReq(t, http.MethodGet, "/api/v1/audit-log", nil, apiKey)
	w := execAuthed(t, d, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /audit-log: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var entries []map[string]any
	decodeJSON(t, w, &entries)
}

// tinyPNG is a valid 1x1 PNG used for upload happy-path tests.
var tinyPNG = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
	0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
	0x89, 0x00, 0x00, 0x00, 0x0a, 0x49, 0x44, 0x41,
	0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
	0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00,
	0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae,
	0x42, 0x60, 0x82,
}

// buildUpload constructs a multipart/form-data body with a single "file"
// field containing the given bytes and filename. Matches the form field
// name HandleUpload reads via r.FormFile("file").
func buildUpload(t *testing.T, filename string, data []byte) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := fw.Write(data); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}
	return &buf, mw.FormDataContentType()
}

// TestUpload_SmallPNG uploads a minimal 1x1 PNG and expects the handler to
// write it to var/uploads and return {"url": "/uploads/<hex>.png", "filename": "..."}.
//
// HandleUpload does not read the advertiser from context; it only inspects
// the "file" form field and the filename extension. execPublic is sufficient.
//
// The test removes the written file via t.Cleanup so the worktree doesn't
// accumulate QA leftovers. The upload dir is cwd-relative (tracked as a P5
// concern — see the bug list).
func TestUpload_SmallPNG(t *testing.T) {
	d := mustDeps(t)
	_, apiKey := newAdvertiser(t, d)

	buf, ctype := buildUpload(t, "tiny.png", tinyPNG)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/upload", buf)
	req.Header.Set("Content-Type", ctype)
	req.Header.Set("X-API-Key", apiKey)

	w := execPublic(t, d, req)
	if w.Code != http.StatusOK && w.Code != http.StatusCreated {
		t.Fatalf("POST /upload (png): expected 200/201, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		URL      string `json:"url"`
		Filename string `json:"filename"`
	}
	decodeJSON(t, w, &resp)
	if !strings.HasPrefix(resp.URL, "/uploads/") {
		t.Fatalf("POST /upload (png): expected url to start with /uploads/, got %q", resp.URL)
	}
	if !strings.HasSuffix(resp.URL, ".png") {
		t.Fatalf("POST /upload (png): expected url to end with .png, got %q", resp.URL)
	}
	if resp.Filename == "" {
		t.Fatalf("POST /upload (png): expected non-empty filename, got body %s", w.Body.String())
	}

	// Clean up the uploaded blob so repeated runs don't leak files.
	t.Cleanup(func() {
		_ = os.Remove(filepath.Join("var", "uploads", resp.Filename))
	})
}

// TestUpload_RejectNonImage uploads a file with an unsupported extension
// (.exe) and expects the handler to reject it. HandleUpload checks the
// extension against allowedExts (jpg/jpeg/png/gif/webp) and returns 400
// on mismatch — see upload.go:72-76.
func TestUpload_RejectNonImage(t *testing.T) {
	d := mustDeps(t)
	_, apiKey := newAdvertiser(t, d)

	buf, ctype := buildUpload(t, "evil.exe", []byte("MZ\x90\x00this is not an image"))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/upload", buf)
	req.Header.Set("Content-Type", ctype)
	req.Header.Set("X-API-Key", apiKey)

	w := execPublic(t, d, req)
	if w.Code != http.StatusBadRequest && w.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("POST /upload (exe): expected 400/415, got %d: %s", w.Code, w.Body.String())
	}
}

// TestUpload_RejectMislabeled uploads non-image bytes under an allowed
// filename extension (.png). Extension-only validation would accept it
// — the P2.5b upload hotfix adds content-based MIME sniffing via
// http.DetectContentType so mislabeled blobs are rejected before hitting
// var/uploads.
func TestUpload_RejectMislabeled(t *testing.T) {
	d := mustDeps(t)
	_, apiKey := newAdvertiser(t, d)

	// Same non-image bytes used in TestUpload_RejectNonImage, now wearing
	// a .png filename. The handler must not trust the filename.
	buf, ctype := buildUpload(t, "evil.png", []byte("MZ\x90\x00this is not an image"))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/upload", buf)
	req.Header.Set("Content-Type", ctype)
	req.Header.Set("X-API-Key", apiKey)

	w := execPublic(t, d, req)
	if w.Code != http.StatusBadRequest && w.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("POST /upload (mislabeled .png): expected 400/415, got %d: %s", w.Code, w.Body.String())
	}
}
