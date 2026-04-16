package handler

import (
	"bytes"
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

// allowedMIMEs is the content-type allowlist enforced by HandleUpload via
// http.DetectContentType sniffing on the first 512 bytes. Must stay in
// sync with allowedExts (jpg/jpeg share image/jpeg).
var allowedMIMEs = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/gif":  true,
	"image/webp": true,
}

// UploadFileServer serves uploaded files from var/uploads/.
func UploadFileServer() http.Handler {
	return http.FileServer(http.Dir(uploadDir))
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

	// First-line filter: reject obviously-disallowed extensions fast.
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if !allowedExts[ext] {
		WriteError(w, http.StatusBadRequest, fmt.Sprintf("unsupported file type: %s (allowed: jpg, png, gif, webp, svg)", ext))
		return
	}

	// Sniff the first 512 bytes to validate the actual content type.
	// A client cannot be trusted to label its own uploads — e.g. a PHP or
	// JS blob named "evil.png" would otherwise land under /uploads/ and be
	// served to future visitors. http.DetectContentType implements the
	// WHATWG mime-sniff algorithm and reliably identifies standard image
	// formats from their leading bytes.
	sniffBuf := make([]byte, 512)
	n, err := io.ReadFull(file, sniffBuf)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		WriteError(w, http.StatusBadRequest, "failed to read upload")
		return
	}
	sniffBuf = sniffBuf[:n]
	contentType := http.DetectContentType(sniffBuf)
	// DetectContentType may include a charset suffix; trim to the media type.
	if i := strings.Index(contentType, ";"); i >= 0 {
		contentType = strings.TrimSpace(contentType[:i])
	}
	if !allowedMIMEs[contentType] {
		WriteError(w, http.StatusBadRequest, fmt.Sprintf("unsupported file content: %s (allowed: image/jpeg, image/png, image/gif, image/webp)", contentType))
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

	// Re-stream: write the sniffed prefix first, then the remainder of the
	// multipart body. io.MultiReader avoids buffering the whole file in
	// memory and preserves the 10MB MaxBytesReader cap.
	if _, err := io.Copy(dst, io.MultiReader(bytes.NewReader(sniffBuf), file)); err != nil {
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
