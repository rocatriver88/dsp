package handler

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestUploadFileServerRejectsLegacyDir(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir temp: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(wd)
	})

	// Place a file in the legacy uploads/ directory.
	if err := os.MkdirAll("uploads", 0o755); err != nil {
		t.Fatalf("mkdir legacy upload dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join("uploads", "legacy.png"), []byte("legacy-file"), 0o644); err != nil {
		t.Fatalf("write legacy file: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/legacy.png", nil)
	rr := httptest.NewRecorder()
	UploadFileServer().ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for legacy dir file, got %d (body: %s)", rr.Code, rr.Body.String())
	}
}

func TestUploadFileServerServesVarUploads(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir temp: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(wd)
	})

	// Place a file in var/uploads/.
	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		t.Fatalf("mkdir upload dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(uploadDir, "current.png"), []byte("current-file"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/current.png", nil)
	rr := httptest.NewRecorder()
	UploadFileServer().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if body := rr.Body.String(); body != "current-file" {
		t.Fatalf("expected current file body, got %q", body)
	}
}
