package api

import (
	"net/http"

	"github.com/devcoons/dcalcon/internal/app"
	"github.com/devcoons/dcalcon/internal/config"
	"github.com/devcoons/dcalcon/internal/storage"
)

// New serves the REST API and DAV discovery (principals / well-known).
// Calendar and address-book collections can also be served here; in the split
// compose file Caddy sends those collections to the dedicated DAV services.
func New(store *storage.DB, cfg config.Config) http.Handler {
	return app.CombinedHandler(store, cfg)
}
