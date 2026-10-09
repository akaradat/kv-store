package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type record struct {
	Value struct {
		Count int `json:"count"`
	} `json:"value"`
	Version uint64 `json:"version"`
}

func (c counter) get(ctx context.Context) (record, error) {
	raw, status, err := c.send(ctx, http.MethodGet, c.keyURL, "")
	if err != nil {
		return record{}, err
	}
	switch status {
	case http.StatusOK:
	case http.StatusNotFound:
		return record{}, fmt.Errorf("GET: key %q not found; seed it first with {\"count\":0}", c.cfg.key)
	default:
		return record{}, fmt.Errorf("GET: unexpected status %d", status)
	}
	var rec record
	if err := json.Unmarshal(raw, &rec); err != nil {
		return record{}, fmt.Errorf("GET: decode response: %w", err)
	}
	return rec, nil
}

func (c counter) put(ctx context.Context, url string, count int) (int, error) {
	_, status, err := c.send(ctx, http.MethodPut, url, fmt.Sprintf(`{"count":%d}`, count))
	return status, err
}

func (c counter) send(ctx context.Context, method, url, body string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, strings.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	resp, err := c.cfg.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, err
	}
	return raw, resp.StatusCode, nil
}
