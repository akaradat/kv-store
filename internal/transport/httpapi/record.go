package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/akaradat/kv-store/internal/core/kv"
)

type record struct {
	Key     string          `json:"key"`
	Value   json.RawMessage `json:"value"`
	Version uint64          `json:"version"`
}

func WriteRecord(w http.ResponseWriter, e kv.Entry) {
	writeJSON(w, http.StatusOK, record{Key: e.Key, Value: e.Value, Version: e.Version})
}
