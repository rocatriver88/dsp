package handler

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestUploadFileServerFallsBackToLegacyDir(t *testing.T) {
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

	if err := os.MkdirAll(legacyUploadDir, 0o755); err != nil {
		t.Fatalf("mkdir legacy upload dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyUploadDir, "legacy.png"), []byte("legacy-file"), 0o644); err != nil {
		t.Fatalf("write legacy file: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/legacy.png", nil)
	rr := httptest.NewRecorder()
	UploadFileServer().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if body := rr.Body.String(); body != "legacy-file" {
		t.Fatalf("expected legacy file body, got %q", body)
	}
}
