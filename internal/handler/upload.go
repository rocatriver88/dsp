package handler

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const (
	uploadDir     = "var/uploads"
	maxUploadSize = 10 << 20 // 10MB
)

var allowedExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true,
}

// HandleUpload godoc
// @Summary Upload creative image
// @Tags creatives
// @Security ApiKeyAuth
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "Image file"
// @Success 200 {object} object{url=string}
// @Router /upload [post]
func (d *Deps) HandleUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)

	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		WriteError(w, http.StatusBadRequest, "file too large (max 10MB)")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		WriteError(w, http.StatusBadRequest, "missing file field")
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if !allowedExts[ext] {
		WriteError(w, http.StatusBadRequest, fmt.Sprintf("unsupported file type: %s (allowed: jpg, png, gif, webp, svg)", ext))
		return
	}

	// Generate random filename to prevent collisions and path traversal
	randBytes := make([]byte, 16)
	if _, err := rand.Read(randBytes); err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to generate filename")
		return
	}
	filename := hex.EncodeToString(randBytes) + ext

	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to create upload directory")
		return
	}

	dst, err := os.Create(filepath.Join(uploadDir, filename))
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to save file")
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to write file")
		return
	}

	// Return the URL path relative to the API server
	url := fmt.Sprintf("/uploads/%s", filename)
	WriteJSON(w, http.StatusOK, map[string]string{
		"url":      url,
		"filename": filename,
	})
}
