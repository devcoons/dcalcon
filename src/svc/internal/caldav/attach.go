package caldav

import (
	"net/http"
	"strings"

	"github.com/devcoons/dcalcon/internal/auth"
	"github.com/devcoons/dcalcon/internal/httpx"
	"github.com/devcoons/dcalcon/internal/storage"
)

func ServeAttachment(store *storage.DB, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	p, ok := auth.PrincipalFrom(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/dav/attachments/"), "/")
	if !storage.ValidAttachmentPublicID(id) {
		http.NotFound(w, r)
		return
	}
	att, err := store.AttachmentByPublicID(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if _, err := store.CalendarByID(r.Context(), p.ID, att.CalendarID); err != nil {
		http.NotFound(w, r)
		return
	}
	if r.Method == http.MethodHead {
		httpx.WriteDownloadHead(w, att.Filename, att.ContentType, att.Data)
		return
	}
	httpx.WriteDownload(w, att.Filename, att.ContentType, att.Data)
}
