package httpx

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/devcoons/dcalcon/internal/storage"
)

func WriteDownload(w http.ResponseWriter, filename, contentType string, data []byte) {
	SetDownloadHeaders(w, filename, contentType, data)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func WriteDownloadHead(w http.ResponseWriter, filename, contentType string, data []byte) {
	SetDownloadHeaders(w, filename, contentType, data)
	w.WriteHeader(http.StatusOK)
}

func SetDownloadHeaders(w http.ResponseWriter, filename, contentType string, data []byte) {
	name := SafeDownloadName(filename)
	ct := storage.DownloadContentType(contentType, data)
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
}

func SafeDownloadName(filename string) string {
	filename = storage.SanitizeFilename(filename)
	filename = strings.NewReplacer(`"`, "", `\`, "", "\r", "", "\n", "").Replace(filename)
	if filename == "" || filename == "." || filename == ".." {
		return "attachment"
	}
	return filename
}

func DispositionAttachment(w http.ResponseWriter, filename string) {
	w.Header().Set("Content-Disposition", `attachment; filename="`+SafeDownloadName(filename)+`"`)
}
