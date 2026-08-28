package worker

import (
	"net/http"

	"github.com/devcoons/dcalcon/internal/storage"
	iworker "github.com/devcoons/dcalcon/internal/worker"
)

func Handler(store *storage.DB) http.Handler {
	return iworker.Handler(store)
}
