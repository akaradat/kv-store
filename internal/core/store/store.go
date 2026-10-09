package store

import (
	"sync"
	"sync/atomic"

	"github.com/akaradat/kv-store/internal/core/kv"
)

type Store struct {
	entries sync.Map // key → *entry; never removed, so Range needs no lock
}

type entry struct {
	mu   sync.Mutex               // serializes writers to this key; readers never take it
	snap atomic.Pointer[snapshot] // nil until the first write lands → Get reports not found
}

// snapshot is immutable once published, so a reader never sees half a write (R-CON-3).
type snapshot struct {
	value   []byte
	version uint64
}

func New() *Store {
	return &Store{}
}

func (s *Store) Get(key string) (kv.Entry, error) {
	e, ok := s.load(key)
	if !ok {
		return kv.Entry{}, kv.ErrNotFound
	}
	snap := e.snap.Load()
	if snap == nil {
		return kv.Entry{}, kv.ErrNotFound
	}
	return kv.Entry{Key: key, Value: snap.value, Version: snap.version}, nil
}

// Put keeps value; the caller must not modify it afterwards.
func (s *Store) Put(key string, ifVersion uint64, value []byte) (kv.Entry, error) {
	return s.update(key, ifVersion, func(*snapshot) ([]byte, error) {
		return value, nil
	})
}

// Patch may keep delta; the caller must not modify it afterwards.
func (s *Store) Patch(key string, ifVersion uint64, delta []byte) (kv.Entry, error) {
	return s.update(key, ifVersion, func(cur *snapshot) ([]byte, error) {
		if cur == nil {
			return delta, nil
		}
		return kv.Merge(cur.value, delta)
	})
}

// Range doesn't block writers. Keys created while it runs may or may not be visited (R-LIST-6).
func (s *Store) Range(fn func(key string) bool) {
	s.entries.Range(func(k, v any) bool {
		if v.(*entry).snap.Load() == nil {
			return true
		}
		return fn(k.(string))
	})
}

// compute runs under the key's lock, so it must not do I/O (S4).
func (s *Store) update(key string, ifVersion uint64, compute func(cur *snapshot) ([]byte, error)) (kv.Entry, error) {
	e, ok := s.load(key)
	if !ok {
		if ifVersion != kv.Unconditional {
			// R-IFV-4: a failed guard must not create the key.
			return kv.Entry{}, &kv.VersionConflictError{}
		}
		actual, _ := s.entries.LoadOrStore(key, &entry{})
		e = actual.(*entry)
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	cur := e.snap.Load()
	var current uint64
	if cur != nil {
		current = cur.version
	}
	if ifVersion != kv.Unconditional && ifVersion != current {
		return kv.Entry{}, &kv.VersionConflictError{Current: current}
	}
	value, err := compute(cur)
	if err != nil {
		return kv.Entry{}, err
	}
	next := &snapshot{value: value, version: current + 1}
	e.snap.Store(next)

	return kv.Entry{Key: key, Value: next.value, Version: next.version}, nil
}

func (s *Store) load(key string) (*entry, bool) {
	v, ok := s.entries.Load(key)
	if !ok {
		return nil, false
	}
	return v.(*entry), true
}
