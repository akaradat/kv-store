package router

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/akaradat/kv-store/internal/core/cluster"
	"github.com/akaradat/kv-store/internal/transport/httpapi"
)

const copyBufferBytes = 32 << 10

var errIncomplete = errors.New("node stream ended without a final newline")

// fanOut opens every stream before sending 200, so a down node gives 503, not a partial list (M7).
// A non-nil error means the 200 was already sent and the caller must abort the connection.
func (h *handler) fanOut(w http.ResponseWriter, r *http.Request) error {
	streams, err := h.openAll(r.Context())
	defer func() {
		for _, s := range streams {
			if s != nil {
				s.Close()
			}
		}
	}()
	if err != nil {
		h.log.Warn("list: node unavailable", "err", err)
		httpapi.WriteError(w, httpapi.NodeUnavailable())
		return nil
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.WriteHeader(http.StatusOK)
	if err := http.NewResponseController(w).Flush(); err != nil {
		return err
	}
	return copyHoldingLastNewline(flushingWriter{w}, streams)
}

// flushingWriter sends each chunk at once: an abort discards whatever is still buffered, and the client
// must at least see the 200 and the lines copied so far.
type flushingWriter struct {
	w http.ResponseWriter
}

func (f flushingWriter) Write(p []byte) (int, error) {
	n, err := f.w.Write(p)
	if err != nil {
		return n, err
	}
	return n, http.NewResponseController(f.w).Flush()
}

func (h *handler) openAll(ctx context.Context) ([]*nodeStream, error) {
	nodes := h.cluster.Nodes()
	streams := make([]*nodeStream, len(nodes))
	errs := make([]error, len(nodes))
	var wg sync.WaitGroup
	for i, n := range nodes {
		wg.Go(func() {
			streams[i], errs[i] = h.open(ctx, n)
		})
	}
	wg.Wait()
	return streams, errors.Join(errs...)
}

func (h *handler) open(ctx context.Context, n cluster.Node) (*nodeStream, error) {
	ctx, cancel := context.WithCancel(ctx)
	timer := time.AfterFunc(h.timeout, cancel)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, n.URL+"/kv", nil)
	if err != nil {
		cancel()
		return nil, err
	}
	resp, err := h.client.Do(req)
	if !timer.Stop() {
		err = errors.Join(err, fmt.Errorf("%s: no headers within %s", n.ID, h.timeout))
	}
	if err != nil {
		if resp != nil {
			resp.Body.Close()
		}
		cancel()
		return nil, fmt.Errorf("%s: %w", n.ID, err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		cancel()
		return nil, fmt.Errorf("%s: status %d", n.ID, resp.StatusCode)
	}
	return &nodeStream{body: resp.Body, timer: timer, timeout: h.timeout, cancel: cancel}, nil
}

type nodeStream struct {
	body    io.ReadCloser
	timer   *time.Timer // cancels the request when it fires; armed only while a Read waits
	timeout time.Duration
	cancel  context.CancelFunc
}

func (s *nodeStream) Read(p []byte) (int, error) {
	s.timer.Reset(s.timeout)
	n, err := s.body.Read(p)
	s.timer.Stop()
	return n, err
}

func (s *nodeStream) Close() {
	s.timer.Stop()
	s.cancel()
	s.body.Close()
}

// copyHoldingLastNewline delays each stream's final '\n' until more data or a clean end arrives, so a
// listing that fails partway never ends with '\n' and can't be mistaken for a complete one (M7).
func copyHoldingLastNewline(w io.Writer, streams []*nodeStream) error {
	buf := make([]byte, copyBufferBytes)
	held := false
	for _, s := range streams {
		complete := true
		for {
			n, err := s.Read(buf)
			if n > 0 {
				chunk := buf[:n]
				if held {
					if _, err := w.Write([]byte{'\n'}); err != nil {
						return err
					}
				}
				held = chunk[n-1] == '\n'
				complete = held
				if held {
					chunk = chunk[:n-1]
				}
				if _, err := w.Write(chunk); err != nil {
					return err
				}
			}
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return err
			}
		}
		if !complete {
			return errIncomplete
		}
	}
	if held {
		_, err := w.Write([]byte{'\n'})
		return err
	}
	return nil
}
