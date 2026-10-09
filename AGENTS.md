# AGENTS.md

In-memory key–value HTTP service in Go 1.27, built as a take-home in three parts: single node, multi-node
(router + nodes), and a design-only roadmap. Two production binaries: `kvnode` (stores data) and
`kvrouter` (routes keys to nodes, fans out `GET /kv`). Production code uses the stdlib plus chi (router).

## Commands

```bash
make check      # gofmt + vet + staticcheck + layer rules (scripts/check-arch.sh) + test -race. The "am I done?" gate.
make test       # go test -race ./...
make e2e        # real binaries as processes (-tags e2e); slower, not part of check
make run-node   # one node on :7000
make counter    # required 3×100 counter test against URL (default :7000); seed the key first, see verify.md §2
make lost-update # the same run without ifVersion: shows lost updates
make debug-node # kvnode under Delve on :2345 (debug-router: kvrouter against nodes on :7001–:7003)
docker compose up --build -d   # 3 nodes + router on :7000; README lists the other docker commands

go test -race ./internal/core/store                             # one package
go test -race ./internal/core/store -ginkgo.focus="stale guard" # one spec
go test -race ./... -ginkgo.label-filter=R-IFV-6                # specs for one requirement
```

## Layout

```
cmd/{kvnode,kvrouter}             wiring only: flags, env, files, http.Server
internal/core/                    INSIDE: no net/http, no I/O
  kv/        rules: Entry, errors, ValidateKey, Merge. Imports nothing internal.
  store/     per-key lock + sync.Map + atomic snapshots. Its methods ARE the usecases.
  cluster/   topology parse, node IDs, CRC32 → 1024 slots → node. Takes []byte, not a path.
internal/transport/               OUTSIDE: speaks HTTP
  httpapi/   shared by node AND router: parse, validate, envelope, errors → status, request log
  node/      kvnode handler; declares the only interface, `Store`
  router/    forward, node-failure mapping, GET /kv fan-out
internal/servertest/              test helper: in-process node, router, cluster on httptest
test/integration/                 whole-system specs: router + real nodes in one process; always run
test/e2e/                         real binaries as processes; all files tagged //go:build e2e (make e2e)
demo/                             presentation clients for verify.md §2 (seed/verify by curl); not product binaries
  kvcounter/    required 3×100 counter test: concurrent guarded increments; no tests of its own
  kvlostupdate/ negative control: the same increments without ifVersion, shows lost updates
```

Layer rules (`make check` enforces them): `core` never imports `transport` or `net/http`; `core/kv` imports
nothing internal. New code goes where
[`docs/standard/structure.md` §4](docs/standard/structure.md#4-where-new-code-goes) says.

## Source of truth

`docs/requirements/brief/` > `requirement.md` > `decisions.md` > `solution.md` > code.

Each part has `docs/requirements/key-store-value-{single,multi}-node/`:
- `requirement.md`: behaviour, with IDs (`R-KEY`, `R-VER`, `R-IFV`, `R-BODY`, `R-CON`, `R-ERR`, `R-FID`,
  `R-LIST`, `R-NODE`, `R-PATCH`, `R-HTTP`, `R-MN-*`).
- `decisions.md`: why each choice was made (S1–S7, M1–M8).
- `solution.md`: how it's built.
- `verify.md`: how each behaviour is checked.

**If code and a spec disagree, or a spec is ambiguous, stop and ask.** Never edit a spec to match the code.

## Conventions

Full list with reasons: [`docs/standard/conventions.md`](docs/standard/conventions.md). The ones agents get
wrong:

- **Readability and maintainability over performance.** Use the stdlib or a well-known library rather than
  hand-written code; write custom code only when no library meets a requirement, and name that
  requirement in a one-line comment.
- **Comments say what the code can't:** why, invariants, what a lock guards, the requirement ID. No comments
  that restate the code, narrate a change, or label sections. Prefer a better name over a comment.
- Domain errors live in `core/kv`. Only `httpapi` maps errors to status codes. `core` never logs.
- `store` methods take no `context.Context`. Constructors return concrete types.
- A constructor with parameters takes one struct named after it plus `Input`:
  `NewHandler(in NewHandlerInput)`, `cluster.New(in NewInput)`. No-parameter constructors stay `New()`.
- Tests use Ginkgo v2 + Gomega only: one `<pkg>_suite_test.go` per package, `Label("R-…")` on every spec,
  `DescribeTable` for matrices, `DeferCleanup` for servers, no mocks.

## Gotchas

- **Never `json.Marshal` a response body.** It escapes `<>&` inside stored strings. Use `httpapi`'s
  `writeJSON` (`json.Encoder`, `SetEscapeHTML(false)`), with the stored value as `json.RawMessage` (S3).
- **Never hold a lock during I/O.** Inside the lock: load → check `ifVersion` → compute → publish (S4).
- **Node and router each own their chi routes** (`NewHandler` in `node` and in `router`); they share only
  `httpapi` validation and writers. A key route must name its segment `{key}` for `httpapi.ParseKey`, and
  the router must use `middleware.RequestSize(httpapi.MaxBodyBytes)` or `ParseWrite` never returns 413. No
  `http.ServeMux`, and no `r.URL.Query()` for `ifVersion`: it silently drops a malformed `ifVersion` and
  turns a guarded write into an unguarded one (S1).
- **Guarded write on a missing key → 409 with `currentVersion: null`, and no entry is created.**
- **An entry with no snapshot yet is a 404** and is skipped by `GET /kv`.
- **Goroutines in specs:** collect results, assert after `wg.Wait()`. Any goroutine that calls `Expect`
  starts with `defer GinkgoRecover()`.
- The router validates requests with the **same** `httpapi` code as a node (M5), so a bad request gets
  400/413 even when the owning node is down.

## Boundaries

**Always**
- Run `make check` before saying a task is done, and show the tail of its output.
- Add or update a labelled spec for every requirement ID you touch.
- Update `solution.md` when behaviour or structure changes.

**Ask first**
- Changing any `requirement.md` or `decisions.md`.
- Adding a package under `internal/`.
- Changing the wire format (envelope, error JSON, NDJSON) or the slot count / `cluster.json` shape.

**Never**
- Create `pkg/`, `util/`, `utils/`, `common/`, `mocks/`, `models/`, or a usecase/service layer.
- Read, cite or edit `prep/`. It holds private notes and competing designs that are not this project's.
- Implement Part 3. It is design-only (`docs/requirements/key-store-value-roadmap/roadmap.md`).
- Commit or push unless asked.

## Definition of done

1. `make check` passes, and the output is shown.
2. Every requirement ID touched has a labelled spec.
3. Docs match the code (`solution.md`; `decisions.md` only with approval).
4. No new folders outside the layout.

After changing Go files, `go.mod` or the Makefile, run `make check` and fix every failure before handing the
work back.
