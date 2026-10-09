# Multi-node KV Store: Verification

How to check that a running deployment meets [`requirement.md`](requirement.md):

1. Run the single-node curl checks through the endpoint (§1).
2. Check `GET /kv` across nodes (§2).
3. Check behavior when a node is down (§3).
4. Run the key-spread test (§4).
5. Run the concurrent counter test through the endpoint (§5).

Assumes a fresh deployment of **3 nodes** (`node-1`, `node-2`, `node-3`) with the client endpoint on
`http://localhost:7000`. How to start it is defined in `solution.md`. Response JSON is shown compact.
Field order and whitespace may differ.

---

## 1. Same API through the endpoint

Run every step of the single-node
[`verify.md` §1](../key-store-value-single-node/verify.md#1-basic-usage-with-curl) against
`http://localhost:7000`, **unchanged**. Every status and body must match what that file expects
(R-MN-API-1, R-MN-API-2). Responses for `/kv/{key}` must not contain a node ID (R-MN-API-4).

Then write keys that will land on different nodes, for use in the next sections:

```bash
for k in a b c d e f g h i j; do
  curl -s -X PUT http://localhost:7000/kv/$k -d "{\"name\":\"$k\"}"; echo
done
```
Expect `200` and `version` 1 for each.

---

## 2. `GET /kv` across nodes

```bash
curl -i http://localhost:7000/kv
```

Expect:
- `200`, `Content-Type: application/x-ndjson`.
- One line per key, in any order. Lines from different nodes may be mixed together:
  ```
  {"key":"a","node":"node-2"}
  {"key":"b","node":"node-1"}
  {"key":"c","node":"node-3"}
  ...
  ```
- Every key written so far appears **exactly once** (R-MN-LIST-1, R-MN-LIST-3).
- At least two different `node` values appear (10 keys over 3 nodes).
- The body ends with `\n`.

**The listed owner is stable** (R-MN-TOP-4): run `GET /kv` again. Each key shows the same `node` as before.

**Empty deployment** (R-MN-LIST-7): on a freshly started deployment, `curl -i http://localhost:7000/kv` →
`200` with an empty body.

---

## 3. A node is down

Use `GET /kv` to pick a key owned by `node-2` (say `a`) and one owned by another node (say `b`). Then
stop the `node-2` process (with Docker: `docker compose kill node-2`).

| Command | Expect |
|---|---|
| `curl -i localhost:7000/kv/a` | `503` `{"error":"node_unavailable","message":"..."}` |
| `curl -i -X PUT localhost:7000/kv/a -d '{"x":1}'` | `503` `node_unavailable` (not applied) |
| `curl -i -X PATCH localhost:7000/kv/a -d '{"x":1}'` | `503` `node_unavailable` |
| `curl -i localhost:7000/kv/b` | `200`. Healthy nodes are unaffected (R-MN-FAIL-1) |
| `curl -i -X PATCH localhost:7000/kv/b -d '{"x":1}'` | `200`, `version` 2 |
| `curl -i -X PUT localhost:7000/kv/a -d '[1]'` | `400` `invalid_body`. Stateless validation still answers (R-MN-FAIL-5) |
| `curl -i localhost:7000/kv` | `503` `node_unavailable`, and **no** partial list (R-MN-LIST-6) |

Restart `node-2` (no other change; with Docker: `docker compose start node-2`):

| Command | Expect |
|---|---|
| `curl -i localhost:7000/kv/a` | `404`. The node restarted empty (R-MN-FAIL-4) |
| `curl -i localhost:7000/kv/b` | `200`, `version` 2 (other nodes kept their data) |
| `curl -i localhost:7000/kv` | `200`, lists keys from all 3 nodes again |

> 504 `node_timeout` and 502 `bad_node_response` need a node that hangs or misbehaves. They aren't part of
> the manual check. If they're tested at all, it's in automated tests with a stub node.

---

## 4. Key spread test

Checks R-MN-TOP-3, R-MN-TOP-5. It runs as two Ginkgo specs rather than a script: `core/cluster` puts
30,000 seeded random keys through the slot table, and `integration` writes 30,000 keys through an
in-process router to 3 nodes and checks `GET /kv` (`go test ./... -ginkgo.label-filter=R-MN-TOP-5`). This
section defines what they check.

**Inputs:** endpoint URL, number of keys = **30,000**, 3 nodes, a fresh deployment.

**Steps**
1. Generate 30,000 random valid keys (e.g. `k-<random 16 hex chars>`).
2. `PUT /kv/<key>` with `{}` for each one (in parallel). Every response must be `200`, `version` 1.
3. `GET /kv` and count lines per `node`.

**Pass**
- Total lines == 30,000, and no key appears twice (exactly one owner per key).
- Every node owns **9,000 – 11,000** keys (90–110% of the average).
- The response ends with `\n`.

**Output**, e.g.:
```
node-1: 10034  (100.3%)
node-2:  9921  ( 99.2%)
node-3: 10045  (100.5%)
total: 30000, duplicates: 0  PASS
```

---

## 5. Concurrent counter test through the endpoint

Checks R-MN-CON-1. This is the **same script** as single-node
[`verify.md` §2](../key-store-value-single-node/verify.md#2-concurrent-counter-test-required) (3 clients ×
100 increments using `GET` → `PUT ?ifVersion=v`, retrying on 409), pointed at the endpoint URL.

**Pass:** `count == 300`, `version == 301`, total conflicts > 0, no status other than `200`/`409`.

**Extra run, counters on different nodes in parallel** (R-MN-CON-2): seed candidate keys
`counter-1` … `counter-20` with `{"count":0}`. Use `GET /kv` to pick 3 of them owned by 3 different nodes.
Run the counter test on all 3 keys **at the same time** (3 clients each, 9 in
total). Each key must end with `count == 300`, `version == 301`. The `demo/kvcounter` client takes the key with
`-key`, so run one per key.
