# KV Store

An in-memory key–value HTTP service in Go, built in three parts:

1. **Single node:** per-key atomic `GET` / `PUT` / `PATCH` with optimistic versioning (`ifVersion`).
2. **Multi-node:** a stateless router spreads keys over several nodes (CRC32 → 1024 fixed slots), and
   `GET /kv` lists every key across nodes as NDJSON.
3. **Roadmap:** a design-only plan for the next step.

The brief is in [`docs/requirements/brief/`](docs/requirements/brief/raw-kv-store-assignment.md).

## Requirements

- Go 1.27 and `make`
- Docker with Compose, only to run the 3-node cluster

Production code uses the Go standard library plus the [chi](https://github.com/go-chi/chi) router. Tests use Ginkgo v2 and Gomega.

## Run

```bash
make run-node                    # single node on http://localhost:7000
docker compose up --build -d     # 3 nodes + router on http://localhost:7000 (ready ~1s later)
```

```bash
curl -X PUT http://localhost:7000/kv/user:42 -d '{"name":"Ari","points":10}'
curl http://localhost:7000/kv/user:42
curl -X PUT 'http://localhost:7000/kv/user:42?ifVersion=1' -d '{"name":"Ari","points":20}'
curl -X PATCH http://localhost:7000/kv/user:42 -d '{"rank":"gold"}'
curl http://localhost:7000/kv          # NDJSON: {"key":"user:42","node":"node-2"}
```

## Docker cluster

`compose.yaml` runs 3 `kvnode` containers and a `kvrouter`. The router is on http://localhost:7000, and
each node is also published on http://localhost:7001–7003 so you can read a node directly.

```bash
docker compose up --build -d            # build the image and start (again after a code change)
docker compose ps                       # what is running
docker compose down                     # stop and remove everything (all data is lost)
```

Logs (one text line per request; the router also logs `msg=route key=… slot=… node=…`):

```bash
docker compose logs -f                  # follow all services, each line prefixed with its name
docker compose logs -f router           # follow one service
docker compose logs --tail 20 node-2    # last 20 lines of one node
docker compose logs router | grep msg=route   # which node each key went to
```

Nodes:

```bash
docker compose kill node-2              # crash node-2 (SIGKILL): its keys answer 503
docker compose stop node-2              # or stop it gracefully (SIGTERM)
docker compose start node-2             # bring it back: it starts empty, its old keys answer 404
docker compose restart node-2           # stop + start in one step
curl localhost:7002/kv                  # list what node-2 holds, bypassing the router
```

## Debug

VS Code (`.vscode/launch.json`, needs the Go extension):

| Launch config | What it does |
|---|---|
| kvnode | Runs one node on :7000 under the debugger |
| kvrouter (nodes in Docker) | Runs the router on the host on :7000 with `cluster.json`. Start the nodes first: `docker compose up --build -d node-1 node-2 node-3` |
| Attach to make debug-* | Attaches to a `make debug-node` / `make debug-router` session |

Any editor, through Delve on `127.0.0.1:2345` (the program starts running; attach to set breakpoints):

```bash
make debug-node                                    # kvnode on :7000
docker compose up --build -d node-1 node-2 node-3  # then:
make debug-router                                  # kvrouter on :7000 against those nodes
```

## Test

```bash
make test            # every spec except real-binary e2e, with the race detector
make e2e             # builds the binaries and runs them as processes (nodes, router, counter)
make check           # gofmt, go vet, staticcheck, layer rules, tests
make counter         # required test: 3 clients × 100 guarded increments = 300, against a running server
go run ./test/e2e/kvcounter -no-guard   # same run without ifVersion: shows lost updates
```

## Design

| Topic | Document |
|---|---|
| Single node: behaviour / design / decisions / verification | [`docs/requirements/key-store-value-single-node/`](docs/requirements/key-store-value-single-node/) |
| Multi-node: behaviour / design / decisions / verification | [`docs/requirements/key-store-value-multi-node/`](docs/requirements/key-store-value-multi-node/) |
| Code layout and layer rules | [`docs/standard/structure.md`](docs/standard/structure.md) |
| Code conventions | [`docs/standard/conventions.md`](docs/standard/conventions.md) |
| Part 3 roadmap | `docs/roadmap.md` (not written yet) |
| Working with AI agents | [`AGENTS.md`](AGENTS.md) |

## Status

- [x] Decide stack and Part 1 concurrency model
- [x] Decide Part 2 partitioning and routing
- [x] Project structure, conventions, agent instructions
- [x] Part 1: single node
- [x] Part 1: required test, 3 clients × 100 increments = 300
- [x] Part 2: multi-node and `GET /kv` NDJSON
- [ ] Part 3: roadmap doc and diagram
- [ ] Demo rehearsal (40–50 min)
