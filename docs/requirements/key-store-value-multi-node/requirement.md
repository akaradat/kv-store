# Multi-node KV Store: Requirements

Source of truth for **what** a multi-node KV Store deployment must do. It says nothing about **how**
(router vs. other designs, hashing scheme, transport). Those belong in `solution.md`.

This file **extends** the single-node contract,
[`../key-store-value-single-node/requirement.md`](../key-store-value-single-node/requirement.md). Every
rule there (keys, versioning, `ifVersion`, body, PUT/PATCH semantics, errors, concurrency, `GET /kv` format)
still applies, as seen by a client of the deployment. This file only adds what changes with several
nodes. Where the two conflict, this file wins for a multi-node deployment.

Derived from [`brief/raw-kv-store-assignment.md`](../brief/raw-kv-store-assignment.md) (Part 2).

Keywords **MUST**, **MUST NOT**, **SHOULD**, and **MAY** are used as in RFC 2119.

---

## 1. Scope

### In scope
- Several node processes that together store the keyspace, so total key capacity grows with the number
  of nodes.
- **One client-facing endpoint** that serves the full single-node API for the whole keyspace.
- `GET /kv` listing every key across all nodes.
- A fixed node list given as configuration at startup.
- Defined client-visible behavior when a node is down or slow.

### Out of scope
- High availability, replication, failover, and backups. A down node's keys are unavailable, and its data
  is lost when it restarts (in-memory).
- Adding or removing nodes at runtime, rebalancing, resharding, and key migration.
- Making the client-facing endpoint itself highly available. If it is down, the service is down.
- A throughput target. Spreading keys evenly (§4) is required. A measured speed-up is not.
- Talking to a node directly. How nodes behave when called directly is not part of the contract.
- Operations across keys (multi-key transactions, batch writes).
- Topology or admin endpoints (e.g. listing nodes or their health).

---

## 2. Terms

| Term | Meaning |
|---|---|
| **Node** | A process holding part of the keyspace in memory. Each node meets the single-node contract for the keys it owns. |
| **Node ID** | A node's identifier, following single-node R-NODE-2 (`^[A-Za-z0-9_-]{1,64}$`). |
| **Endpoint** | The one base URL that clients use. It may be a separate process or part of a node; that is a solution choice. |
| **Owner** | The single node that stores a given key. |
| **Topology** | The ordered list of node IDs (and their addresses) given in the startup configuration. |

---

## 3. Client view

- **R-MN-API-1 One endpoint.** Clients use a single base URL for every operation: `GET`/`PUT`/`PATCH
  /kv/{key}` and `GET /kv`.
- **R-MN-API-2 Same API.** For every request, the deployment returns the **same status, error code, and
  body** that a single node would return, given the same sequence of operations on that key. That
  includes validation (400/404/405/413), the validation order (single-node R-ERR-4), `ifVersion`/409, and
  value fidelity. The only additions are the statuses in §6.
- **R-MN-API-3 Nodes are hidden.** A client never needs to know how many nodes there are or which one owns
  a key. The `node` field in `GET /kv` is informational.
- **R-MN-API-4** A response for `/kv/{key}` never includes a node ID, as in single-node R-NODE-4.

---

## 4. Topology and key placement

- **R-MN-TOP-1 Static config.** The topology is given as configuration at startup (N ≥ 1 nodes; the demo
  uses 3) and does not change while the deployment runs. Changing it means restarting the deployment.
- **R-MN-TOP-2 Valid config.** Startup MUST fail if the topology is empty, contains a duplicate node ID,
  or contains an invalid node ID.
- **R-MN-TOP-3 Exactly one owner.** Each key is stored on **exactly one** node. No key is stored on two
  nodes, and a key is never stored on a node other than its owner.
- **R-MN-TOP-4 Deterministic.** The owner is decided only by the key and the topology. The same key with
  the same topology always maps to the same node, including across restarts.
- **R-MN-TOP-5 Even spread.** With 30,000 random keys and 3 nodes, every node owns **between 90% and 110%**
  of the average (10,000), so 9,000–11,000 keys each.
- **R-MN-TOP-6 N = 1.** A deployment with one node behaves exactly like the single-node contract.

---

## 5. Per-key semantics across the deployment

- **R-MN-CON-1** Every single-node concurrency rule (R-CON-1 … R-CON-6) holds for clients of the endpoint.
  In particular: per-key atomicity, no torn reads, no lost updates, and exactly one winner for concurrent
  writes with the same `ifVersion`.
- **R-MN-CON-2** Operations on keys owned by **different** nodes MUST NOT wait on each other, except for
  incidental, bounded contention at the endpoint.
- **R-MN-CON-3** Versions are per key, as in single-node §4. Versions of keys on different nodes are not
  related to each other.

---

## 6. Node failure and timeouts

Three new error statuses. Error bodies follow single-node R-ERR-1 exactly (`error` + `message`, no extra
fields; no node ID, per R-MN-API-3):

```json
{ "error": "node_unavailable", "message": "..." }
```

| Status | `error` | When | Was the request applied? |
|---|---|---|---|
| **503** | `node_unavailable` | The owning node can't be reached (e.g. the connection is refused) **before** the request is sent to it | **No.** Safe to retry. |
| **504** | `node_timeout` | The request was sent to the owner, but no full reply arrived in time (timeout or connection dropped) | **Unknown.** It may or may not have been applied |
| **502** | `bad_node_response` | The owner replied with something that is not a valid response | **Unknown** |

- **R-MN-FAIL-1 Isolation.** A down or slow node affects **only** the keys it owns. Requests for keys on
  healthy nodes behave normally.
- **R-MN-FAIL-2 No automatic retry.** The deployment MUST NOT retry a `PUT` or `PATCH` on its own (a retry
  could apply the write twice: version +2, or a duplicate merge). Clients that need certainty after a 504
  can `GET` the key, or retry with `ifVersion` (a stale guard gives 409, so the retry is safe).
- **R-MN-FAIL-3 Configurable timeout.** The per-request timeout to a node is set in configuration.
- **R-MN-FAIL-4 No failover.** A down node's keys are not served by any other node. When the node comes
  back it is empty, and its keys behave as never written (`GET` → 404).
- **R-MN-FAIL-5** Validation that doesn't depend on stored state (bad key, bad `ifVersion`, bad body,
  oversized body, wrong method) is still answered with its single-node 4xx even when the owner is down.

---

## 7. `GET /kv` across nodes

Format, field shape, and the trailing-`\n` rule are the same as single-node §7.4. Additionally:

- **R-MN-LIST-1 All nodes.** The response lists the keys of **every** node in the topology.
- **R-MN-LIST-2 Owner in `node`.** Each line's `node` is the ID of the node that owns and stores that key.
- **R-MN-LIST-3 Once.** Each key appears exactly once across the whole response. (This follows from
  R-MN-TOP-3.)
- **R-MN-LIST-4 Order.** Unspecified. Lines from different nodes may interleave.
- **R-MN-LIST-5 Consistency.** Weakly consistent, as in single-node R-LIST-6, across all nodes. Every key
  that existed on any node when the request started is listed. Keys created during the listing may or may
  not appear. Listing MUST NOT block writes on any node for its whole duration.
- **R-MN-LIST-6 Never a silent partial list.**
  - If any node is unreachable **before** the response starts → **503** `node_unavailable` and no
    list.
  - If a node fails **after** the 200 has been sent → the response ends without a trailing `\n` on its
    last line, so it is detectably incomplete.
  - So a 200 whose body ends with `\n` (or is empty) always means "every key on every node".
- **R-MN-LIST-7** An empty deployment → 200 with an empty body.

---

## 8. Configuration

- **R-MN-CFG-1** Configuration at startup includes: the topology (node IDs and their addresses), the
  endpoint's listen address, and the node request timeout (R-MN-FAIL-3).
- **R-MN-CFG-2** Each node's own node ID (single-node §2a) MUST match its ID in the topology, so that
  `GET /kv` reports the correct owner.
- The format and mechanism (file, flags, env) are defined in `solution.md`.

---

## 9. Verification

How to verify a deployment is in [`verify.md`](verify.md).

---

## 10. Decision log

| # | Decision | Rationale |
|---|---|---|
| M1 | One client-facing endpoint, nodes hidden | Clients and curl demos stay unchanged from single-node. Routing stays a server-side concern. |
| M2 | Static topology from startup config, one owner per key, deterministic | Fits the timebox. Rebalancing is a roadmap topic. Deterministic placement makes tests repeatable. |
| M3 | A down node fails only its own keys (503) | Partial availability is the point of scaling out. HA itself is out of scope. |
| M4 | `GET /kv` never returns a silent partial list | A 200 that ends cleanly always means a complete list. A failure is either a 503 or a visibly truncated stream. |
| M5 | 503 = not applied, 504/502 = unknown, no automatic write retry | Retrying could apply a write twice. Clients can recover safely with `GET` or `ifVersion`. |
| M6 | Required: even spread (90–110% of average). Not required: throughput target | Spread is deterministic and testable. Throughput on one machine is noisy. |
| M7 | Stateless validation still returns 4xx when the owner is down | Same answer for the same bad request regardless of node health. |
