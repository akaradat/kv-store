package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/akaradat/kv-store/internal/core/kv"
)

type Error struct {
	Status  int
	Code    string
	Message string
}

func (e *Error) Error() string {
	return e.Code + ": " + e.Message
}

func notFound() *Error {
	return &Error{Status: http.StatusNotFound, Code: "not_found", Message: "no such route"}
}

func invalidKey(err error) *Error {
	return &Error{Status: http.StatusBadRequest, Code: "invalid_key", Message: err.Error()}
}

func invalidIfVersion(msg string) *Error {
	return &Error{Status: http.StatusBadRequest, Code: "invalid_if_version", Message: msg}
}

func invalidBody(msg string) *Error {
	return &Error{Status: http.StatusBadRequest, Code: "invalid_body", Message: msg}
}

func bodyTooLarge() *Error {
	return &Error{Status: http.StatusRequestEntityTooLarge, Code: "body_too_large", Message: "body must be at most " + strconv.Itoa(MaxBodyBytes) + " bytes"}
}

// NodeUnavailable, NodeTimeout and BadNodeResponse never name the node: clients don't see the topology
// (R-MN-API-3).
func NodeUnavailable() *Error {
	return &Error{Status: http.StatusServiceUnavailable, Code: "node_unavailable", Message: "the owning node could not be reached; the request was not applied"}
}

func NodeTimeout() *Error {
	return &Error{Status: http.StatusGatewayTimeout, Code: "node_timeout", Message: "no full reply from the owning node in time; the request may or may not have been applied"}
}

func BadNodeResponse() *Error {
	return &Error{Status: http.StatusBadGateway, Code: "bad_node_response", Message: "the owning node sent an invalid reply; the request may or may not have been applied"}
}

type errorBody struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

type conflictBody struct {
	Error          string  `json:"error"`
	Message        string  `json:"message"`
	CurrentVersion *uint64 `json:"currentVersion"`
}

// WriteError is the only place an error becomes a status code.
func WriteError(w http.ResponseWriter, err error) {
	var pe *Error
	var conflict *kv.VersionConflictError
	switch {
	case errors.As(err, &pe):
		writeJSON(w, pe.Status, errorBody{Error: pe.Code, Message: pe.Message})
	case errors.Is(err, kv.ErrNotFound):
		writeJSON(w, http.StatusNotFound, errorBody{Error: "not_found", Message: err.Error()})
	case errors.As(err, &conflict):
		body := conflictBody{Error: "version_conflict", Message: err.Error()}
		if conflict.Current != 0 {
			body.CurrentVersion = &conflict.Current
		}
		writeJSON(w, http.StatusConflict, body)
	default:
		writeJSON(w, http.StatusInternalServerError, errorBody{Error: "internal_error", Message: err.Error()})
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	var body bytes.Buffer
	enc := json.NewEncoder(&body)
	// json.Marshal would rewrite <, > and & inside stored strings as \u escapes (R-FID-1).
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", strconv.Itoa(body.Len()))
	w.WriteHeader(status)
	_, _ = w.Write(body.Bytes())
}
