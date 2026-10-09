package node

import (
	"bufio"
	"encoding/json"
	"net/http"

	"github.com/akaradat/kv-store/internal/transport/httpapi"
)

func (h *handler) get(w http.ResponseWriter, r *http.Request) {
	key, err := httpapi.ParseKey(r)
	if err != nil {
		httpapi.WriteError(w, err)
		return
	}
	entry, err := h.store.Get(key)
	if err != nil {
		httpapi.WriteError(w, err)
		return
	}
	httpapi.WriteRecord(w, entry)
}

func (h *handler) put(w http.ResponseWriter, r *http.Request) {
	in, err := httpapi.ParseWrite(r)
	if err != nil {
		httpapi.WriteError(w, err)
		return
	}
	entry, err := h.store.Put(in.Key, in.IfVersion, in.Body)
	if err != nil {
		httpapi.WriteError(w, err)
		return
	}
	httpapi.WriteRecord(w, entry)
}

func (h *handler) patch(w http.ResponseWriter, r *http.Request) {
	in, err := httpapi.ParseWrite(r)
	if err != nil {
		httpapi.WriteError(w, err)
		return
	}
	entry, err := h.store.Patch(in.Key, in.IfVersion, in.Body)
	if err != nil {
		httpapi.WriteError(w, err)
		return
	}
	httpapi.WriteRecord(w, entry)
}

const listBufferBytes = 32 << 10

type listLine struct {
	Key  string `json:"key"`
	Node string `json:"node"`
}

// list streams inside Range instead of collecting keys first, so memory stays constant (S6).
func (h *handler) list(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.WriteHeader(http.StatusOK)

	out := bufio.NewWriterSize(w, listBufferBytes)
	enc := json.NewEncoder(out)
	enc.SetEscapeHTML(false)
	h.store.Range(func(key string) bool {
		err := enc.Encode(listLine{Key: key, Node: h.nodeID})
		return err == nil && r.Context().Err() == nil
	})
	_ = out.Flush()
}
