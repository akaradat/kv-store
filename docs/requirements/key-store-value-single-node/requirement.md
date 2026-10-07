# Single-node KV Store: Requirements

Source of truth for **what** a single KV Store node must do. It says nothing about **how** (language,
locking strategy, data structures). Those belong in `solution.md`.

This file is the complete contract for one node, whether it runs alone or as one node of a multi-node
deployment. A multi-node deployment reuses this contract per node. Its extra rules (routing, aggregating
`GET /kv` across nodes) live in [`../key-store-value-multi-node/requirement.md`](../key-store-value-multi-node/requirement.md).

Items tagged **(part 2)** exist to support a multi-node deployment (brief Part 2). A node still
implements them when it runs alone. Untagged items come from brief Part 1.

Derived from [`brief/raw-kv-store-assignment.md`](../brief/raw-kv-store-assignment.md). Where the
brief is silent or ambiguous, the decision recorded here wins.

Keywords **MUST**, **MUST NOT**, **SHOULD**, and **MAY** are used as in RFC 2119.

---

## 1. Scope

### In scope
- A single-process HTTP API that stores JSON object values by string key, in memory only.
- Operations: `GET`, `PUT`, `PATCH` on `/kv/{key}`, and `GET /kv` (list this node's keys) **(part 2)**.
- A node ID that identifies the node (§2a) **(part 2)**.
- A version number per key, with optimistic concurrency via `ifVersion`.
- Per-key atomicity and concurrency safety.

### Out of scope
- Persistence, durability, and restart recovery (data is lost when the process stops).
- `DELETE` or any other way to remove a key or a field.
- Routing, partitioning, and aggregating keys across nodes (see the multi-node requirements) **(part 2)**.
- Pagination, filtering, sorting, or returning values in `GET /kv` **(part 2)**.
- Server-side increment or any other operation not listed here.
- `If-Match` / `ETag` headers and any header-based conditional request.
- Version history (only the current version is kept).
- TTL, eviction, and a cap on the number of keys. Behavior when the process runs out of memory is undefined.
- Authentication, authorization, TLS, rate limiting.
- High availability, replication.

---

## 2. Data model

| Concept | Definition |
|---|---|
| **Key** | A string matching the rules in §3. Case-sensitive. |
| **Value** | A JSON **object** (`{...}`). Never an array, scalar, or `null` at the top level. Nested values inside the object may be any JSON type. |
| **Version** | A positive integer per key. See §4. |
| **Record** | The triple `{ key, value, version }`. |
| **Node ID** **(part 2)** | A string identifying this node. See §2a. |

## 2a. Node ID (part 2)

- **R-NODE-1** The node ID is set at startup (by a command-line flag or environment variable) and does
  not change while the process runs.
- **R-NODE-2** It MUST match `^[A-Za-z0-9_-]{1,64}$`. An invalid value makes startup fail.
- **R-NODE-3** If no ID is given, it defaults to `node-1`.
- **R-NODE-4** It appears only in `GET /kv` output (§7.4). Responses for `/kv/{key}` do not include it.
- Making node IDs unique across a deployment is the multi-node layer's job.

---

## 3. Keys

- **R-KEY-1** A key MUST match `^[A-Za-z0-9:_.-]{1,256}$`.
- **R-KEY-2** A key is the raw path segment after `/kv/`. It is **not** percent-decoded, so any `%` in the
  segment makes the key invalid.
- **R-KEY-3** Keys `.` and `..` are invalid. Other dot combinations are valid (`...`, `.x`, `a.b`).
- **R-KEY-4** Keys are case-sensitive: `User:1` and `user:1` are different keys.
- **R-KEY-5** An invalid key → **400**.
- **R-KEY-6** A path with more than one segment after `/kv/` (e.g. `/kv/a/b`, or the trailing slash in
  `/kv/a/`) does not match the route → **404**.
- **R-KEY-7** An empty key (`/kv/`) → **400**.

> If an HTTP client rewrites the path before sending (for example, curl collapsing `/kv/..`), the service
> handles the request it actually receives.

---

## 4. Versioning

- **R-VER-1** Versions are per key. A key's first successful write sets version **1**.
- **R-VER-2** Every successful `PUT` or `PATCH` raises the version by exactly **1**. This includes:
  - a write whose value is identical to the current value,
  - `PUT {}` and `PATCH {}`.
- **R-VER-3** A failed request (any 4xx) MUST NOT change the value or the version.
- **R-VER-4** A version never decreases or resets. There is no delete, so a key, once created, exists until
  the process stops.
- **R-VER-5** Versions are integers in the range `1 .. 2^63-1`. Overflow is out of scope.

---

## 5. `ifVersion` (optimistic concurrency guard)

- **R-IFV-1** `ifVersion` is accepted **only** as a query parameter: `?ifVersion=<n>`. Headers such as
  `If-Match` are ignored.
- **R-IFV-2** Format: MUST match `^[1-9][0-9]*$` and fit in a signed 64-bit integer. Anything else → **400**,
  including:
  - empty (`?ifVersion=`), `0`, negative, leading zero (`01`), sign (`+1`), decimal (`1.0`), exponent
    (`1e2`), whitespace, or overflow (> `9223372036854775807`),
  - the parameter given more than once (`?ifVersion=1&ifVersion=2`), even with equal values.
- **R-IFV-3** If `ifVersion` is present and valid, and the key exists with a different version → **409**.
- **R-IFV-4** If `ifVersion` is present and valid, and the key **does not exist** → **409** (a missing key
  has no version, so no `ifVersion` can match). Nothing is created.
- **R-IFV-5** If `ifVersion` is absent, the write is unconditional.
- **R-IFV-6** The version check and the write it guards MUST be atomic. No other write to the same key may
  happen between them.
- **R-IFV-7** `ifVersion` on `GET` is ignored and not validated.
- **R-IFV-8** Unknown query parameters are ignored on every endpoint.

---

## 6. Request body (`PUT` and `PATCH`)

- **R-BODY-1** The body MUST be exactly one JSON value per RFC 8259, UTF-8 encoded. Whitespace before and
  after it is allowed.
- **R-BODY-2** That value MUST be a JSON **object**. Array, string, number, boolean, or `null` → **400**.
- **R-BODY-3** Each of these → **400**:
  - an empty body or a whitespace-only body,
  - malformed JSON,
  - trailing data after the object (`{}{}`, `{} x`),
  - invalid UTF-8, or a leading byte-order mark (BOM),
  - nesting deeper than **64** levels (the top-level object counts as 1; objects and arrays each count
    as a level).
- **R-BODY-4** A body larger than **1 MiB** (1,048,576 bytes) → **413**.
- **R-BODY-5** The `Content-Type` request header is ignored. The body is parsed as JSON regardless.
- **R-BODY-6** An empty object `{}` is a valid body.
- **R-BODY-7** Duplicate field names (e.g. `{"name":1,"name":2}`) are **accepted**. See §8 for what is
  stored.

---

## 7. Endpoints

All responses have `Content-Type: application/json`, except a successful `GET /kv`
(`application/x-ndjson`, §7.4) **(part 2)**.

### 7.1 `GET /kv/{key}`

| Case | Status | Body |
|---|---|---|
| Key exists | **200** | `{ "key", "value", "version" }` |
| Key does not exist | **404** | error body (§9) |
| Invalid key | **400** | error body |

### 7.2 `PUT /kv/{key}[?ifVersion=n]`: replace

| Case | Status | Effect |
|---|---|---|
| Key missing, no `ifVersion` | **200** | Create with body as value, version 1 |
| Key missing, `ifVersion` given | **409** | None |
| Key exists, no `ifVersion` | **200** | Replace whole value, version +1 |
| Key exists, `ifVersion` == current | **200** | Replace whole value, version +1 |
| Key exists, `ifVersion` != current | **409** | None |

The response body on 200 is the record **as committed by this request**:
`{ "key", "value", "version" }`.

### 7.3 `PATCH /kv/{key}[?ifVersion=n]`: shallow merge / upsert

| Case | Status | Effect |
|---|---|---|
| Key missing, no `ifVersion` | **200** | Create with body (`delta`) as value, version 1 |
| Key missing, `ifVersion` given | **409** | None |
| Key exists, guard passes or absent | **200** | Shallow merge, version +1 |
| Key exists, `ifVersion` != current | **409** | None |

Shallow merge rules:
- **R-PATCH-1** For each top-level field in `delta`, set it on the stored object, overwriting any existing
  field with the same name. Fields not in `delta` are left unchanged.
- **R-PATCH-2** Merging is **top-level only**. A nested object in `delta` replaces the existing field
  as a whole and is not merged recursively.
- **R-PATCH-3** A field set to `null` in `delta` is stored as `null`. It does **not** delete the field.
  Fields cannot be removed with PATCH; use PUT to remove them.
- **R-PATCH-4** `PATCH {}` on an existing key leaves the value semantically unchanged and still raises the
  version by 1.
- The brief's "otherwise treat as replace" branch (non-object value or delta) cannot happen, because both
  stored values and bodies are always objects.

The response body on 200 is the record as committed by this request.

### 7.4 `GET /kv`: list keys (part 2)

Lists every key stored on **this node**.

- **R-LIST-1** Response: **200**, `Content-Type: application/x-ndjson`.
- **R-LIST-2** One line per key, in the form `{"key":"<key>","node":"<node-id>"}`, and every line ends in
  `\n`, including the last. A body whose last line has no `\n` was cut off.
- **R-LIST-3** Each key appears **at most once**. Values and versions are not included.
- **R-LIST-4** Order is **unspecified**. Clients must not depend on it.
- **R-LIST-5** An empty store → **200** with an empty body (zero lines).
- **R-LIST-6** **Weakly consistent** under concurrent writes:
  - every key that existed when the request started is listed (keys are never deleted),
  - a key created while the listing is running may or may not appear,
  - the listing MUST NOT block writes for its whole duration.
- **R-LIST-7** No query parameters are defined (no pagination, prefix, or limit). Unknown parameters are
  ignored.
- **R-LIST-8** `/kv/` (trailing slash) is **not** the list endpoint. It is `/kv/{key}` with an empty key →
  400 (R-KEY-7).
- If the connection drops while the listing streams, the client gets a truncated body (see R-LIST-2).
  The service does not resume it.

Example:
```
{"key":"user:42","node":"node-1"}
{"key":"counter","node":"node-1"}
```

### 7.5 Other methods and paths

- **R-HTTP-1** Any method other than `GET`, `PUT`, `PATCH` on `/kv/{key}` (e.g. `DELETE`, `POST`) → **405**,
  with an `Allow: GET, PUT, PATCH` header. **(part 2)** Any method other than `GET` on
  `/kv` → **405**, with `Allow: GET`.
- **R-HTTP-2** Any path not defined here (e.g. `/`, `/kv/a/b`) → **404**.
- **R-HTTP-3** Creating a key returns **200**, not 201. The client can tell a create from `version: 1`.

---

## 8. Value fidelity (what GET returns)

- **R-FID-1** After a `PUT`, or a `PATCH` that **creates** the key, `value` is the **exact JSON text** of the
  request body, minus surrounding whitespace. Field order, number text (e.g. `12345678901234567890`, `1.0`),
  string escapes, inner whitespace, and duplicate field names are preserved.
- **R-FID-2** After a `PATCH` **merge**, `value` is only guaranteed to be **semantically equal** to the merge
  result. Field order, whitespace, number formatting, and escapes are not guaranteed.
- **R-FID-3** Duplicate field names in a merge (in the stored value or in `delta`) have **undefined**
  behavior: which occurrence wins, and whether duplicates survive, is not specified. The result MUST still
  be a valid JSON object, the version MUST still go up by 1, and atomicity still applies.
- **R-FID-4** `key` in a response is the key exactly as it appears in the request path.

---

## 9. Errors

- **R-ERR-1** Every 4xx response has the body:
  ```json
  { "error": "<code>", "message": "<human-readable detail>" }
  ```
- **R-ERR-2** A **409** response also includes `"currentVersion"`: the key's current version, or `null` if
  the key does not exist.
  ```json
  { "error": "version_conflict", "message": "...", "currentVersion": 7 }
  ```
- **R-ERR-3** Error codes:

| Status | `error` | When |
|---|---|---|
| 400 | `invalid_key` | §3 |
| 400 | `invalid_if_version` | §5 R-IFV-2 |
| 400 | `invalid_body` | §6 |
| 404 | `not_found` | GET of a missing key; unknown path |
| 405 | `method_not_allowed` | §7.5 |
| 409 | `version_conflict` | §5 R-IFV-3, R-IFV-4 |
| 413 | `body_too_large` | §6 R-BODY-4 |

- **R-ERR-4** Validation order. The first failure decides the response:
  1. route / method → 404 / 405
  2. key → 400
  3. `ifVersion` format → 400
  4. body size → 413
  5. body syntax / type / depth → 400
  6. version guard → 409

  So a request that is both malformed and stale gets **400**, not 409.

---

## 10. Concurrency and consistency

- **R-CON-1 Per-key atomicity.** Operations on the same key take effect one at a time, in some total
  order. Each operation sees the full result of every operation ordered before it.
- **R-CON-2 Independent keys.** Operations on different keys MUST NOT wait on each other, except for
  incidental, bounded contention.
- **R-CON-3 No torn reads.** A `GET` returns a `value` and `version` that were committed together by the
  same write. It never returns a partially applied write or a value from one write with the version of
  another.
- **R-CON-4 No lost updates.** Every successful write is reflected in the state seen by later operations on
  that key. Two concurrent `PATCH`es with different fields both survive. Two concurrent writes with the
  same `ifVersion` cannot both succeed: exactly one gets 200 and the other gets 409.
- **R-CON-5 Read-after-write.** Once a write's 200 response is sent, any `GET` that starts afterwards sees
  that write or a later one.
- **R-CON-6 Response matches commit.** A write's 200 response contains the value and version that write
  committed, not a later write's state.
- **R-CON-7 Listing does not stall the store (part 2).** A running `GET /kv` MUST NOT stop reads or writes on keys
  for its whole duration. Its consistency guarantee is R-LIST-6.

---

## 11. Verification

How to verify a running node is in [`verify.md`](verify.md): basic `GET` / `PUT` / `PATCH` checks with curl,
and the brief's required concurrent counter test (3 clients × 100 increments = 300).

---

## 12. Edge-case quick reference

| # | Request | Expected |
|---|---|---|
| 1 | `PUT /kv/a` body `[1,2]` | 400 `invalid_body` |
| 2 | `PUT /kv/a` body `"x"` / `42` / `true` / `null` | 400 `invalid_body` |
| 3 | `PUT /kv/a` empty body | 400 `invalid_body` |
| 4 | `PUT /kv/a` body `{} {}` | 400 `invalid_body` |
| 5 | `PUT /kv/a` body `{}` (new key) | 200, value `{}`, version 1 |
| 6 | `PUT /kv/a` body `{}` (existing v3) | 200, value `{}`, version 4 |
| 7 | `PUT /kv/a` same value as current | 200, version +1 |
| 8 | `PUT /kv/a` body `{"name":1,"name":2}` | 200, GET returns `{"name":1,"name":2}` verbatim |
| 9 | `PATCH /kv/a` (stored has dup fields) | 200, version +1, which value wins is undefined |
| 10 | `PATCH /kv/a` body `{}` (existing) | 200, value unchanged, version +1 |
| 11 | `PATCH /kv/a` body `{}` (missing) | 200, value `{}`, version 1 |
| 12 | `PATCH /kv/a` body `{"x":null}` | 200, `x` present with value `null` |
| 13 | `PATCH /kv/a` body `{"o":{"b":1}}` over `{"o":{"a":1}}` | `o` becomes `{"b":1}` |
| 14 | `PUT /kv/a?ifVersion=1` (key missing) | 409, `currentVersion: null` |
| 15 | `PATCH /kv/a?ifVersion=5` (current 4) | 409, `currentVersion: 4` |
| 16 | `?ifVersion=0` / `-1` / `01` / `+1` / `1.0` / `abc` / empty | 400 `invalid_if_version` |
| 17 | `?ifVersion=1&ifVersion=1` | 400 `invalid_if_version` |
| 18 | `?ifVersion=99999999999999999999` | 400 `invalid_if_version` |
| 19 | `GET /kv/a?ifVersion=abc` | ignored. 200 or 404 depending on the key |
| 20 | `If-Match: 3` header on PUT | ignored, write is unconditional |
| 21 | `PUT /kv/a?ifVersion=abc` body `[1]` | 400 `invalid_if_version` (validation order) |
| 22 | `PUT /kv/a?ifVersion=9` (stale) body `[1]` | 400 `invalid_body` (400 beats 409) |
| 23 | Body > 1 MiB | 413 `body_too_large` |
| 24 | Body nested 65 levels deep | 400 `invalid_body` |
| 25 | Body with BOM or invalid UTF-8 | 400 `invalid_body` |
| 26 | `Content-Type: text/plain` with valid object body | 200 (header ignored) |
| 27 | `GET /kv/` | 400 `invalid_key` |
| 28 | `GET /kv/a/b`, `GET /kv/a/` | 404 |
| 29 | `GET /kv/user%3A42` | 400 `invalid_key` (`%` not allowed, no decoding) |
| 30 | `GET /kv/a b`, `GET /kv/ключ`, key of 257 chars | 400 `invalid_key` |
| 31 | `GET /kv/.` or `/kv/..` (as received by server) | 400 `invalid_key` |
| 32 | `GET /kv/User:1` after `PUT /kv/user:1` | 404 (case-sensitive) |
| 33 | `DELETE /kv/a`, `POST /kv/a` | 405, `Allow: GET, PUT, PATCH` |
| 34 (part 2) | `GET /kv` on empty store | 200, `application/x-ndjson`, empty body |
| 34a (part 2) | `GET /kv` after creating `a` and `b` (node ID `node-1`) | 2 lines, any order, each `{"key":…,"node":"node-1"}\n` |
| 34b (part 2) | `GET /kv` after 5 writes to the same key | that key listed once |
| 34c (part 2) | `GET /kv?limit=1&prefix=x` | params ignored, all keys listed |
| 34d (part 2) | `POST /kv`, `DELETE /kv` | 405, `Allow: GET` |
| 34e (part 2) | `GET /kv` while other clients create keys | every key that existed at the start is listed. New keys may or may not be |
| 34f (part 2) | Writes during a long `GET /kv` | they still complete, they don't wait for the listing |
| 35 | PUT `{"n":12345678901234567890,"f":1.0}` then GET | number text returned exactly as sent |

---

## 13. Decision log

| # | Decision | Rationale |
|---|---|---|
| D1 | `ifVersion` in query string only, no `If-Match` | Matches the brief's examples. One mechanism. |
| D2 | `ifVersion` on a missing key → 409 | The brief's rule says "supplied and doesn't match". A missing key has no version. |
| D3 | Per-key version from 1, +1 per successful write | Simple, and lets a client predict the next version. |
| D4 | Strict `ifVersion` parsing, repeated param → 400 | No ambiguity on a concurrency guard. |
| D5 | Key charset `[A-Za-z0-9:_.-]{1,256}`, no decoding, no `.`/`..` | Raw path equals the key, so there are no encoding or normalization surprises. |
| D6 | `Content-Type` ignored | Fewer status codes, and curl demos work without `-H`. |
| D7 | Raw text after PUT, semantic equality after merge | Keeps duplicate fields and number precision on PUT without making merge expensive. |
| D8 | Body: one JSON object, ≤ 1 MiB, depth ≤ 64 | Rejects ambiguous input and bounds memory per request. |
| D9 | PATCH `null` stored literally (not RFC 7396) | The brief says shallow merge. No delete anywhere. |
| D10 | JSON error body, `currentVersion` on 409, 405 + `Allow` | Helps debugging and lets a client retry without an extra GET. |
| D11 | Create returns 200 | Literal reading of the brief. |
| D12 | Counter test uses a CAS loop + negative control (see `verify.md` §2) | No increment API. Shows the guard is what prevents lost updates. |
| D13 (part 2) | `GET /kv` is part of the single-node contract | One spec for a node, whether it runs alone or in a cluster. A multi-node layer aggregates per-node listings. |
| D14 (part 2) | Node ID set at startup, defaults to `node-1` | A single node works with no config. A cluster gives each node its own ID. |
| D15 (part 2) | `GET /kv` is weakly consistent | Doesn't stall writes. Matches what a multi-node fan-out can promise. |
| D16 (part 2) | `GET /kv` unordered, no params, keys only | Matches the brief's format. Sorting would force a merge across nodes. |
