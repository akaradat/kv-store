package kv

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
)

// Merge decodes only the top level, so nested numbers keep their exact text (S5).
func Merge(stored, delta []byte) ([]byte, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(stored, &fields); err != nil {
		return nil, fmt.Errorf("decode stored value: %w", err)
	}
	var overlay map[string]json.RawMessage
	if err := json.Unmarshal(delta, &overlay); err != nil {
		return nil, fmt.Errorf("decode delta: %w", err)
	}
	if fields == nil {
		fields = make(map[string]json.RawMessage, len(overlay))
	}
	maps.Copy(fields, overlay)

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	// json.Marshal would rewrite <, > and & inside stored strings as \u escapes.
	enc.SetEscapeHTML(false)
	if err := enc.Encode(fields); err != nil {
		return nil, fmt.Errorf("encode merged value: %w", err)
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}
