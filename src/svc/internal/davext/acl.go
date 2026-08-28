package davext

import (
	"context"
	"html"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/devcoons/dcalcon/internal/davpath"
	"github.com/devcoons/dcalcon/internal/storage"
)

var aceBlock = regexp.MustCompile(`(?is)<(?:[a-z0-9]+:)?ace\b[^>]*>.*?</(?:[a-z0-9]+:)?ace>`)
var aceHREF = regexp.MustCompile(`(?is)<(?:[a-z0-9]+:)?href(?:\s[^>]*)?>\s*([^<]+)\s*</(?:[a-z0-9]+:)?href>`)

func (h *Handler) handleACL(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != "ACL" {
		return false
	}
	col, ok := parseCollection(r.URL.Path)
	if !ok || col.Kind != "calendar" {
		return false
	}
	p, ok := principalOrDeny(r)
	if !ok || !strings.EqualFold(col.Username, p.Username) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return true
	}
	c, err := h.Store.CalendarBySlug(r.Context(), p.ID, col.Slug)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return true
	}
	if !c.IsOwner() || c.Kind != "personal" {
		http.Error(w, "only the owner can change this ACL", http.StatusForbidden)
		return true
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil || dangerousXML(body) {
		http.Error(w, "invalid xml", http.StatusBadRequest)
		return true
	}
	grants, err := parseACLBody(r.Context(), h.Store, c.UserID, body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return true
	}
	if err := h.Store.ReplaceShares(r.Context(), c.ID, grants); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return true
	}
	w.Header().Set("DAV", "1, 3, access-control, calendar-access, addressbook, calendar-auto-schedule")
	w.WriteHeader(http.StatusOK)
	return true
}

func parseACLBody(ctx context.Context, store *storage.DB, ownerID int64, body []byte) ([]storage.ShareGrant, error) {
	var grants []storage.ShareGrant
	seen := map[int64]bool{}
	for _, block := range aceBlock.FindAll(body, -1) {
		s := strings.ToLower(string(block))
		if strings.Contains(s, "<d:deny") || strings.Contains(s, "<deny") {
			continue
		}
		m := aceHREF.FindSubmatch(block)
		if len(m) < 2 {
			continue
		}
		user, ok := principalFromHREF(string(m[1]))
		if !ok {
			continue
		}
		u, err := store.UserByUsername(ctx, user)
		if err != nil {
			continue
		}
		if u.ID == ownerID {
			continue
		}
		access := "read"
		if strings.Contains(s, ">all<") || strings.Contains(s, "<d:write") || strings.Contains(s, "<write") ||
			strings.Contains(s, "write-content") || strings.Contains(s, "write-acl") {
			access = "write"
		} else if !strings.Contains(s, ">read<") && !strings.Contains(s, "<d:read") && !strings.Contains(s, "<read") {
			continue
		}
		if seen[u.ID] {
			continue
		}
		seen[u.ID] = true
		grants = append(grants, storage.ShareGrant{GranteeID: u.ID, Access: access})
	}
	return grants, nil
}

func principalFromHREF(raw string) (string, bool) {
	path := hrefPath(html.UnescapeString(strings.TrimSpace(raw)))
	path = strings.Trim(path, "/")
	const prefix = "dav/principals/"
	if !strings.HasPrefix(strings.ToLower(path), prefix) {
		return "", false
	}
	rest := strings.Trim(path[len(prefix):], "/")
	if rest == "" || strings.Contains(rest, "/") {
		return "", false
	}
	return rest, true
}

func calendarACLSnippet(c *storage.Calendar, viewer string, shares []storage.CalendarShare) string {
	if c == nil {
		return ""
	}
	owner := c.OwnerUsername
	if owner == "" {
		owner = viewer
	}
	ownerHREF := html.EscapeString(davpath.PrincipalPath(owner))
	var b strings.Builder
	b.WriteString(`<D:owner xmlns:D="DAV:"><D:href>`)
	b.WriteString(ownerHREF)
	b.WriteString(`</D:href></D:owner>`)
	b.WriteString(`<D:supported-privilege-set xmlns:D="DAV:">`)
	b.WriteString(`<D:supported-privilege><D:privilege><D:all/></D:privilege>`)
	b.WriteString(`<D:supported-privilege><D:privilege><D:read/></D:privilege></D:supported-privilege>`)
	b.WriteString(`<D:supported-privilege><D:privilege><D:write/></D:privilege></D:supported-privilege>`)
	b.WriteString(`<D:supported-privilege><D:privilege><D:write-properties/></D:privilege></D:supported-privilege>`)
	b.WriteString(`<D:supported-privilege><D:privilege><D:write-content/></D:privilege></D:supported-privilege>`)
	b.WriteString(`<D:supported-privilege><D:privilege><D:bind/></D:privilege></D:supported-privilege>`)
	b.WriteString(`<D:supported-privilege><D:privilege><D:unbind/></D:privilege></D:supported-privilege>`)
	b.WriteString(`<D:supported-privilege><D:privilege><D:read-acl/></D:privilege></D:supported-privilege>`)
	b.WriteString(`<D:supported-privilege><D:privilege><D:write-acl/></D:privilege></D:supported-privilege>`)
	b.WriteString(`</D:supported-privilege></D:supported-privilege-set>`)
	b.WriteString(`<D:current-user-privilege-set xmlns:D="DAV:">`)
	b.WriteString(privilegeXML(c))
	b.WriteString(`</D:current-user-privilege-set>`)
	b.WriteString(`<D:acl xmlns:D="DAV:">`)
	b.WriteString(`<D:ace><D:principal><D:href>`)
	b.WriteString(ownerHREF)
	b.WriteString(`</D:href></D:principal><D:grant><D:privilege><D:all/></D:privilege></D:grant><D:protected/></D:ace>`)
	for _, s := range shares {
		b.WriteString(`<D:ace><D:principal><D:href>`)
		b.WriteString(html.EscapeString(davpath.PrincipalPath(s.Username)))
		b.WriteString(`</D:href></D:principal><D:grant><D:privilege><D:read/></D:privilege>`)
		if s.Access == "write" {
			b.WriteString(`<D:privilege><D:write/></D:privilege>`)
		}
		b.WriteString(`</D:grant></D:ace>`)
	}
	b.WriteString(`</D:acl>`)
	return b.String()
}

func privilegeXML(c *storage.Calendar) string {
	if c.Kind == "important_dates" || (c.ReadOnly && c.Kind != "inbox") {
		return `<D:privilege><D:read/></D:privilege><D:privilege><D:read-acl/></D:privilege>`
	}
	if c.Kind == "inbox" {
		return `<D:privilege><D:read/></D:privilege><D:privilege><D:unbind/></D:privilege><D:privilege><D:read-acl/></D:privilege>`
	}
	if !c.CanWrite() {
		return `<D:privilege><D:read/></D:privilege><D:privilege><D:read-acl/></D:privilege>`
	}
	out := `<D:privilege><D:read/></D:privilege><D:privilege><D:write/></D:privilege>` +
		`<D:privilege><D:write-properties/></D:privilege><D:privilege><D:write-content/></D:privilege>` +
		`<D:privilege><D:bind/></D:privilege><D:privilege><D:unbind/></D:privilege>` +
		`<D:privilege><D:read-acl/></D:privilege>`
	if c.IsOwner() {
		out += `<D:privilege><D:write-acl/></D:privilege>`
	}
	return out
}
