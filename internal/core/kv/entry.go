package kv

// Unconditional is the ifVersion of a write without a guard. Real versions start at 1 (R-VER-1).
const Unconditional uint64 = 0

type Entry struct {
	Key     string
	Value   []byte // shared with the store's immutable snapshot: never modify
	Version uint64
}
