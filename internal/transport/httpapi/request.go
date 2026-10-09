package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/akaradat/kv-store/internal/core/kv"
)

type Write struct {
	Key       string
	IfVersion uint64
	Body      []byte // surrounding whitespace trimmed (R-FID-1)
}

// ParseKey is the first check after routing (R-ERR-4). The route must name the segment {key}.
func ParseKey(r *http.Request) (string, error) {
	// chi routes on RawPath when the path has escapes, so %3A arrives undecoded (R-KEY-2).
	key := chi.URLParam(r, "key")
	if err := kv.ValidateKey(key); err != nil {
		return "", invalidKey(err)
	}
	return key, nil
}

// ParseWrite runs the PUT/PATCH checks in R-ERR-4 order: key → ifVersion → body size → body.
func ParseWrite(r *http.Request) (Write, error) {
	key, err := ParseKey(r)
	if err != nil {
		return Write{}, err
	}
	ifVersion, err := parseIfVersion(r.URL.RawQuery)
	if err != nil {
		return Write{}, err
	}
	body, err := readBody(r)
	if err != nil {
		return Write{}, err
	}
	return Write{Key: key, IfVersion: ifVersion, Body: body}, nil
}
