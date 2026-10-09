# Code Conventions

Rules that differ from default Go, or where the right choice isn't obvious. `gofmt`, `go vet` and
`staticcheck` cover the rest and are not repeated here. Package layout and layer rules are in
[`structure.md`](structure.md).

---

## 0. Readability over performance

**Readability and maintainability come before performance.** Prefer the standard library or a well-known
library over hand-written code that does the same job, even when the library is slower or allocates more.

- Write custom code only when no library can meet a requirement. Then a one-line comment names the
  requirement or trap that forced it (e.g. `ifVersion` is parsed by hand because `url.ParseQuery` silently
  drops a malformed pair, S1).
- Optimise only for a measured problem, and leave the measurement in the commit or PR.
- If a requirement only exists to justify custom code, question the requirement first (as was done for
  routing: `requirement.md` D17).

## 1. Clean code and comments

The code explains itself through names, small functions and types. A comment is for what the code
**can't** say.

**Write a comment for:**
- **Why**, when the reason isn't visible: a rejected simpler option, a trap in the standard library.
- **Invariants**: what a mutex or atomic guards, what must stay true across calls.
- **The requirement behind a line** whose purpose would otherwise look arbitrary: `// R-IFV-6`.
- **Exported identifiers whose name and signature leave something out**: units, ownership, concurrency
  safety, what a zero value means.

**Don't write a comment that:**
- restates the code (`// increment version`, `// return error`),
- narrates the change (`// added for multi-node`, `// fixed bug`),
- labels sections (`// ---- handlers ----`), or
- is a doc comment that only repeats the name (`// Get gets the value.`).

```go
// Bad: says what the code already says.
// lock the entry
e.mu.Lock()

// Good: says what the code can't.
type entry struct {
	mu   sync.Mutex                // serializes writers to this key; readers never take it
	snap atomic.Pointer[snapshot]  // nil until the first write lands → GET returns 404
}
```

Prefer a better name or a smaller function over a comment. If a block needs a comment to say *what* it
does, extract it into a function whose name says it.

---

## 2. Errors

- Domain errors live in `core/kv`:
  ```go
  var ErrNotFound = errors.New("key not found")

  type VersionConflictError struct {
  	Current uint64 // 0 when the key doesn't exist; versions start at 1 (R-VER), written as null (R-ERR-2)
  }
  ```
- `core` returns these and never knows about status codes. **`httpapi` is the only place that maps an
  error to a status** (`errors.Is` / `errors.As`).
- Validation errors carry the wire code from R-ERR-3 (`invalid_key`, `invalid_if_version`, …), so the
  writer doesn't guess.
- Wrap with `%w` only when adding context the caller needs. Don't wrap just to add a stack of words.
- No `panic` outside startup in `main`, except `panic(http.ErrAbortHandler)`: the stdlib's sentinel for
  aborting a response mid-stream, used once by the router's `GET /kv` fan-out (M7).

## 3. Logging

- `log/slog` with the text handler, created in `main` and passed in. No global logger.
- **`core` never logs.** It returns values and errors.
- `transport` logs one line per request: method, key, status, duration.
- The router also logs `key → slot → node`, which makes routing visible in the demo.

## 4. Construction and state

- `New(...)` returns a concrete type (`*store.Store`). Interfaces are declared by the consumer.
- No package-level mutable state, no `init()`.
- **A constructor (`New`, `NewXxx`) that takes parameters takes exactly one struct, named after the function
  plus `Input`.** Adding a dependency later then means adding a field, not changing every call site.
  ```go
  type NewHandlerInput struct {
  	Store  Store
  	NodeID string
  	Log    *slog.Logger
  }

  func NewHandler(in NewHandlerInput) http.Handler

  node.NewHandler(node.NewHandlerInput{Store: store.New(), NodeID: "node-1", Log: log})
  ```
  - Pass it by value, and set fields by name at the call site.
  - Every field is required unless its comment says what the zero value means. The constructor does not
    fill in hidden defaults.
  - A constructor with no parameters stays `New()` (e.g. `store.New()`). A `context.Context` is never a
    field; it stays a separate first parameter.
  - This applies to constructors only. Other functions keep plain parameters.

## 5. Context

- Transport passes `r.Context()` to the router's node calls, so a client disconnect cancels them.
- `store` methods take **no** `context.Context`. They are in-memory and finish in microseconds; an unused
  ctx is noise.

## 6. Concurrency

- Every mutex and atomic field has a comment saying what it guards.
- **Never hold a lock during I/O** (S4): no network, no response writing, no body reading under a lock.
- Inside the lock: load snapshot → check `ifVersion` → compute → publish. Nothing else.
- Snapshots are immutable once published. Never mutate a value reachable from a published snapshot.

## 7. JSON and stored values

- **Write JSON bodies with `json.Encoder` and `SetEscapeHTML(false)`, never `json.Marshal`** (S3).
  `json.Marshal` rewrites `<`, `>`, `&` inside stored strings. In `httpapi`, go through `writeJSON`.
- A stored value goes into a response as `json.RawMessage`, never decoded and re-encoded.
- Routing is chi. Each binary declares its own routes in its `NewHandler`; a key route names its segment
  `{key}` so `httpapi.ParseKey` can read it, and the router uses `middleware.RequestSize(httpapi.MaxBodyBytes)`
  so `ParseWrite` can answer 413. Don't use `http.ServeMux` (it redirects
  and decodes `%3A`), and don't read `ifVersion` with `r.URL.Query()` or `url.Values` (they silently drop a
  malformed `ifVersion`) (S1).

## 8. Naming and files

- Package names are short nouns that say what the package holds: `store`, `cluster`, `httpapi`.
- No stutter: `kv.Entry`, not `kv.KVEntry`.
- One concern per file, named after what it holds: `merge.go`, `slots.go`, `forward.go`, `fanout.go`.
  No `types.go`, `helpers.go`, `utils.go`.

## 9. Tests (Ginkgo v2 + Gomega)

- Every package with tests has one bootstrap file, `<pkg>_suite_test.go`:
  ```go
  func TestStore(t *testing.T) {
  	RegisterFailHandler(Fail)
  	RunSpecs(t, "store")
  }
  ```
- Ginkgo everywhere; don't mix in plain `func TestX` specs.
- Run with `go test -race ./...`. The `ginkgo` CLI is optional.
- **Label each spec with the requirement IDs it covers:** `It("rejects a stale guard", Label("R-IFV-3"), …)`.
  Then `go test ./... -ginkgo.label-filter=R-IFV-3` runs exactly those specs.
- Use `DescribeTable` / `Entry` for input matrices (parsing, merge, status codes).
- Use `DeferCleanup` for servers and other resources.
- **Spec files and helper files are told apart by name.** A file that only holds helpers starts with
  `helpers` (`helpers_test.go`, or `helpers_<topic>_test.go` when there are several); it has no `It`. Every
  other `_test.go` file, apart from `<pkg>_suite_test.go`, holds specs and is named after what it tests.
- **Concurrency specs:** goroutines only collect results (channels, slices, atomics). All `Expect` calls
  happen on the spec goroutine after `wg.Wait()`. If a goroutine must call `Expect`, its first line is
  `defer GinkgoRecover()`, otherwise a failure panics the whole test binary.
- No mocks. Use the real in-memory store and `internal/servertest`.

## 10. Commits

- Conventional Commits: `feat:`, `fix:`, `test:`, `docs:`, `refactor:`, `chore:`.
- One concern per commit. `make check` passes on every commit.
