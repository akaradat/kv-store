package router

import (
	"net/http"

	"github.com/akaradat/kv-store/internal/transport/httpapi"
)

func (h *handler) get(w http.ResponseWriter, r *http.Request) {
	key, err := httpapi.ParseKey(r)
	if err != nil {
		httpapi.WriteError(w, err)
		return
	}
	h.forward(w, r, key, nil)
}

// M5: checks with the node's own code before forwarding, so a bad request gets its 4xx even when the
// owning node is down (R-MN-FAIL-5).
func (h *handler) write(w http.ResponseWriter, r *http.Request) {
	in, err := httpapi.ParseWrite(r)
	if err != nil {
		httpapi.WriteError(w, err)
		return
	}
	h.forward(w, r, in.Key, in.Body)
}

func (h *handler) list(w http.ResponseWriter, r *http.Request) {
	if err := h.fanOut(w, r); err != nil {
		h.log.Warn("list aborted", "err", err)
		// M7: closing the connection without the final chunk makes the truncation visible to clients
		// that don't parse NDJSON too.
		panic(http.ErrAbortHandler)
	}
}
