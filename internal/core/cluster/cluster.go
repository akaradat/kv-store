package cluster

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"net/url"
	"slices"
)

// NumSlots is fixed forever: changing it moves keys between slots (M2).
const NumSlots = 1024

type Node struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

type Cluster struct {
	nodes []Node
	owner [NumSlots]int // slot → index into nodes; never changes after New, so lookups need no lock
}

// Slot uses CRC32 IEEE so any language computes the same slot (M3).
func Slot(key string) int {
	return int(crc32.ChecksumIEEE([]byte(key)) % NumSlots)
}

func Parse(data []byte) (*Cluster, error) {
	var cfg struct {
		Nodes []Node `json:"nodes"`
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parse cluster config: %w", err)
	}
	return New(NewInput{Nodes: cfg.Nodes})
}

type NewInput struct {
	Nodes []Node // in slot-range order: the first node gets the first range
}

func New(in NewInput) (*Cluster, error) {
	nodes := in.Nodes
	if err := validate(nodes); err != nil {
		return nil, err
	}
	c := &Cluster{nodes: slices.Clone(nodes)}
	base, extra := NumSlots/len(nodes), NumSlots%len(nodes)
	slot := 0
	for i := range nodes {
		size := base
		if i < extra {
			size++
		}
		for range size {
			c.owner[slot] = i
			slot++
		}
	}
	return c, nil
}

func validate(nodes []Node) error {
	if len(nodes) == 0 {
		return errors.New("cluster config: at least one node is required")
	}
	if len(nodes) > NumSlots {
		return fmt.Errorf("cluster config: at most %d nodes", NumSlots)
	}
	seen := make(map[string]bool, len(nodes))
	for _, n := range nodes {
		if err := ValidateNodeID(n.ID); err != nil {
			return fmt.Errorf("cluster config: node %q: %w", n.ID, err)
		}
		if seen[n.ID] {
			return fmt.Errorf("cluster config: duplicate node ID %q", n.ID)
		}
		seen[n.ID] = true
		if u, err := url.Parse(n.URL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return fmt.Errorf("cluster config: node %q: url must be an absolute http(s) URL, got %q", n.ID, n.URL)
		}
	}
	return nil
}

func (c *Cluster) Owner(slot int) Node {
	return c.nodes[c.owner[slot]]
}

func (c *Cluster) Nodes() []Node {
	return slices.Clone(c.nodes)
}
