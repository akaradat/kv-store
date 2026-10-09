package router

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptrace"
	"strconv"
	"sync/atomic"

	"github.com/akaradat/kv-store/internal/core/cluster"
	"github.com/akaradat/kv-store/internal/transport/httpapi"
)

var nodeStatuses = map[int]bool{
	http.StatusOK:                    true,
	http.StatusBadRequest:            true,
	http.StatusNotFound:              true,
	http.StatusMethodNotAllowed:      true,
	http.StatusConflict:              true,
	http.StatusRequestEntityTooLarge: true,
}

// forward sends the request to the key's owner. body is nil for GET. It never retries: a resent write
// could raise the version twice (R-MN-FAIL-2).
func (h *handler) forward(w http.ResponseWriter, r *http.Request, key string, body []byte) {
	slot := cluster.Slot(key)
	owner := h.cluster.Owner(slot)
	h.log.Info("route", "key", key, "slot", slot, "node", owner.ID)

	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	var sent, answered atomic.Bool
	ctx = httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{
		// The transport may retry a request it never wrote; each attempt starts from GetConn.
		GetConn: func(string) {
			sent.Store(false)
			answered.Store(false)
		},
		WroteRequest: func(info httptrace.WroteRequestInfo) {
			sent.Store(info.Err == nil)
		},
		GotFirstResponseByte: func() {
			answered.Store(true)
		},
	})

	target := owner.URL + r.URL.EscapedPath()
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	out, err := http.NewRequestWithContext(ctx, r.Method, target, reader)
	if err != nil {
		httpapi.WriteError(w, err)
		return
	}

	resp, err := h.client.Do(out)
	if err != nil {
		httpapi.WriteError(w, classify(ctx, sent.Load(), answered.Load()))
		return
	}
	defer resp.Body.Close()
	// Read the whole reply before answering: once the client has a 200, a node dropping halfway
	// could no longer be reported as 504.
	reply, err := io.ReadAll(resp.Body)
	if err != nil {
		httpapi.WriteError(w, httpapi.NodeTimeout())
		return
	}
	if !nodeStatuses[resp.StatusCode] {
		httpapi.WriteError(w, httpapi.BadNodeResponse())
		return
	}

	for _, name := range []string{"Content-Type", "Allow"} {
		if v := resp.Header.Get(name); v != "" {
			w.Header().Set(name, v)
		}
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(reply)))
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(reply)
}

func classify(ctx context.Context, sent, answered bool) *httpapi.Error {
	switch {
	case !sent:
		return httpapi.NodeUnavailable()
	case ctx.Err() != nil || !answered:
		return httpapi.NodeTimeout()
	default:
		return httpapi.BadNodeResponse()
	}
}
