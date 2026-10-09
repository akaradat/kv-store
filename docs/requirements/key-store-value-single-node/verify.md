# Single-node KV Store: Verification

How to check that a running node meets [`requirement.md`](requirement.md):

1. Check basic `GET` / `PUT` / `PATCH` by hand with curl (§1).
2. Run the concurrent counter test (§2).

Assumes the node is running on `http://localhost:7000` with an empty store. Response JSON is shown compact.
Field order and whitespace in the envelope may differ.

---

## 1. Basic usage with curl

Run the steps in order. Each step builds on the one before it.

### 1.1 Create with PUT

```bash
curl -i -X PUT http://localhost:7000/kv/user:42 -d '{"name":"Ari","points":10}'
```
Expect `200`:
```json
{"key":"user:42","value":{"name":"Ari","points":10},"version":1}
```

### 1.2 Read with GET

```bash
curl -i http://localhost:7000/kv/user:42
```
Expect `200`, same record as 1.1 (`version` 1).

```bash
curl -i http://localhost:7000/kv/nope
```
Expect `404`:
```json
{"error":"not_found","message":"..."}
```

### 1.3 Replace with PUT (whole value replaced)

```bash
curl -i -X PUT http://localhost:7000/kv/user:42 -d '{"name":"Ari"}'
```
Expect `200`. `points` is gone:
```json
{"key":"user:42","value":{"name":"Ari"},"version":2}
```

### 1.4 Conditional PUT with `ifVersion`

```bash
# matches current version (2) → succeeds
curl -i -X PUT 'http://localhost:7000/kv/user:42?ifVersion=2' -d '{"name":"Ari","points":20}'
```
Expect `200`, `version` 3.

```bash
# stale version → rejected, nothing changes
curl -i -X PUT 'http://localhost:7000/kv/user:42?ifVersion=2' -d '{"name":"Ari","points":99}'
```
Expect `409`:
```json
{"error":"version_conflict","message":"...","currentVersion":3}
```

### 1.5 Merge with PATCH

```bash
curl -i -X PATCH http://localhost:7000/kv/user:42 -d '{"rank":"gold","points":30}'
```
Expect `200`. Existing fields are kept, new or changed fields are applied:
```json
{"key":"user:42","value":{"name":"Ari","points":30,"rank":"gold"},"version":4}
```

```bash
# conditional PATCH, stale version → 409, currentVersion 4
curl -i -X PATCH 'http://localhost:7000/kv/user:42?ifVersion=3' -d '{"rank":"silver"}'
```

### 1.6 PATCH creates a missing key

```bash
curl -i -X PATCH http://localhost:7000/kv/user:43 -d '{"name":"Bo"}'
```
Expect `200`:
```json
{"key":"user:43","value":{"name":"Bo"},"version":1}
```

### 1.7 Common rejections

| Command | Expect |
|---|---|
| `curl -i -X PUT localhost:7000/kv/user:42 -d '[1,2]'` | `400` `invalid_body` (not an object) |
| `curl -i -X PUT localhost:7000/kv/user:42 -d 'null'` | `400` `invalid_body` |
| `curl -i -X PUT localhost:7000/kv/user:42 -d '{bad'` | `400` `invalid_body` |
| `curl -i -X PUT 'localhost:7000/kv/user:42?ifVersion=0' -d '{}'` | `400` `invalid_if_version` |
| `curl -i -X PUT 'localhost:7000/kv/new-key?ifVersion=1' -d '{}'` | `409`, `currentVersion: null`; the key is still not created |
| `curl -i -X PUT 'localhost:7000/kv/bad%20key' -d '{}'` | `400` `invalid_key` |
| `curl -i -X DELETE localhost:7000/kv/user:42` | `405`, `Allow` headers for `GET`, `PUT`, `PATCH` (any order), empty body |

After the rejections, `curl localhost:7000/kv/user:42` still returns `version` 4. Failed requests change
nothing.

### 1.8 List keys (part 2)

```bash
curl -i http://localhost:7000/kv
```
Expect `200`, `Content-Type: application/x-ndjson`, one line per key (any order):
```
{"key":"user:42","node":"node-1"}
{"key":"user:43","node":"node-1"}
```

---

## 2. Concurrent counter test (required)

The brief's required test: **3 concurrent clients × 100 increments = 300**. The increments are run by the
demo client `demo/kvcounter` (`make counter`). Seeding and verifying are done with curl, so each step can be
checked by hand.

### What it checks

Concurrent read-modify-write on one key loses no updates when clients use `ifVersion`. This covers
per-key atomicity (R-CON-1), no lost updates (R-CON-4), and atomic check-and-write (R-IFV-6).

### Steps

1. **Start the server** with an empty store:
   ```bash
   make run-node                  # or: docker compose up --build -d  (router + 3 nodes on :7000)
   ```
2. **Seed** the key:
   ```bash
   curl -i -X PUT http://localhost:7000/kv/counter-with-ifversion -d '{"count":0}'
   ```
   Expect `200`:
   ```json
   {"key":"counter-with-ifversion","value":{"count":0},"version":1}
   ```
3. **Run the clients concurrently:**
   ```bash
   make counter                   # go run ./demo/kvcounter -url http://localhost:7000
   ```
   The script starts all clients at the same moment, so their requests really overlap. Each client repeats
   until it has **100 successful** increments:
   1. `GET /kv/counter-with-ifversion` → read `count = n` and `version = v`.
   2. `PUT /kv/counter-with-ifversion?ifVersion=v` with `{"count": n+1}`.
   3. `200` → one increment done. `409` → someone else wrote first: count a conflict and go back to step 1.
      Any other status → the script fails right away.

   It waits for all clients to finish, then prints successes and conflicts per client.
4. **Verify** the key:
   ```bash
   curl -i http://localhost:7000/kv/counter-with-ifversion
   ```
   Expect `200`:
   ```json
   {"key":"counter-with-ifversion","value":{"count":300},"version":301}
   ```

The store is in memory: to run again, restart the server, or seed a fresh key and pass it with `-key`.

### Pass criteria

- `value.count == 300` (clients × increments).
- `version == 301` (1 seed write + 300 successful increments).
- The script exits 0: every client finished with exactly 100 successes and got no status other than `200`
  or `409`.

### Negative control (`demo/kvlostupdate`)

The same steps on a second key, with `demo/kvlostupdate`, whose `PUT` has **no** `ifVersion`:
```bash
curl -i -X PUT http://localhost:7000/kv/counter-without-ifversion -d '{"count":0}'
make lost-update               # go run ./demo/kvlostupdate -url http://localhost:7000
curl -i http://localhost:7000/kv/counter-without-ifversion
```
The expected result is `version == 301` but `count < 300`: every write was accepted, but clients overwrote
each other's increments. This shows the pass above comes from the `ifVersion` guard, not from lucky timing.
It is a demonstration, not a failure.
