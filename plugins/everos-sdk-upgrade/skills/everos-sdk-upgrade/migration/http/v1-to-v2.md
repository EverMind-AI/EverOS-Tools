# Migration Rules: EverOS Cloud API v1 -> v2 (transport level)

Applies to **any** caller that speaks HTTP to the EverOS Cloud API directly — curl,
Python `requests`/`httpx`, JS `fetch`/`axios`, Go `net/http`, Java, PHP, shell scripts —
in any language, with or without an SDK.

This file is also the **semantic source of truth** for the language-specific SDK rule
files (`../python/v1-to-v2.md`, and future Go/TS ones). Those files map SDK method
signatures onto the wire changes described here; when the two disagree, this file wins.

Apply rules in the order listed.

## If your client is typed (TypeScript, Go, Java, Rust...)

Read this first. Every table below reads like a call-site diff; for a typed client it is a
**type-definition diff first**, and the call sites follow.

- **The request/response type layer is the first edit, not the last.** Nothing compiles
  until it is right, and a half-edited type layer takes down packages that never touched
  EverOS.
- **A flagged call site must still compile.** Where a rule says "flag, do not rewrite", the
  flagged code still references v1 types you are renaming. Re-declare those shapes local to
  the flagged module, prefixed `Legacy`, rather than leaving a dangling reference. Python's
  "move the import into the function body" has no equivalent here.
- **Do not bump a shared version constant.** See API-001.
- **There is no `data` unwrap on the wire.** The Python facade returns `.data`
  pre-unwrapped; that is an SDK convenience (SDK-011) and does not apply to you. On the
  wire `data` stays exactly where it is — only `request_id` moves up. Applying the SDK rule
  here breaks every response parse.

## Preconditions (check before touching any code)

1. **The account must be v2-enabled.** A v1-only account gets `403 VERSION_NOT_ALLOWED`
   on every `/api/v2/*` call. Confirm with the EverOS team before migrating.
2. **The API key does NOT change.** The same key authenticates both v1 and v2 — no
   re-issue, no config change. (Verified live on prod, 2026-09-04.)
3. **v1 keeps working.** v2 access is additive; v1 endpoints are unaffected. Migration
   is opt-in and can be staged.
4. **Data does NOT carry over.** See API-016 — this is the single most important
   planning constraint and it is not a code change.

## Contents

- API-001: Endpoint path map
- API-002: Authentication (unchanged — informational)
- API-003: `add` request body — COMPLETE REWRITE
- API-004: `timestamp` must be unix MILLISECONDS — hard 422
- API-005: New `app_id` / `project_id` scope (new concept, no v1 equivalent)
- API-006: `get` / `search` scoping — `filters` object -> top-level `user_id` / `agent_id`
- API-007: `memory_type` value renames
- API-008: Response envelope + field renames
- API-009: `delete` — status code, body, and scope semantics
- API-010: Error response body shape
- API-011: Extraction timing — sync `add` now extracts, `flush` returns `no_extraction`
- API-012: REMOVED — group memory (no v2 equivalent) — **FLAG, do not rewrite**
- API-013: REMOVED — sender registry (no v2 equivalent) — **FLAG, do not rewrite**
- API-014: REMOVED — memory-space settings (no v2 equivalent) — **FLAG, do not rewrite**
- API-015: Agent memory folded into the unified `add`
- API-016: Data does not carry over — cutover planning (not a code change)
- API-017: New in v2 (informational)
- API-018: Async task polling — the task id moved and the status values changed
- Applying the rules: order and hazards

---

## API-001: Endpoint path map

### Change Type: BREAKING - Path Rename

Note the singular `memory` in v2 (v1 used plural `memories`). This trips up
search-and-replace that only swaps `v1` for `v2`.

| v1 | v2 | Notes |
|---|---|---|
| `POST /api/v1/memories` | `POST /api/v2/memory/add` | Body rewritten — see API-003 |
| `POST /api/v1/memories/agent` | `POST /api/v2/memory/add` | Same endpoint now — see API-015 |
| `POST /api/v1/memories/flush` | `POST /api/v2/memory/flush` | Keyed by `session_id`, not `user_id` |
| `POST /api/v1/memories/agent/flush` | `POST /api/v2/memory/flush` | Same endpoint now |
| `POST /api/v1/memories/get` | `POST /api/v2/memory/get` | Body rewritten — see API-006 |
| `POST /api/v1/memories/search` | `POST /api/v2/memory/search` | Body rewritten — see API-006 |
| `POST /api/v1/memories/delete` | `POST /api/v2/memory/delete` | See API-009 |
| `POST /api/v1/object/sign` | `POST /api/v2/object/sign` | Path-only change |
| `GET /api/v1/tasks/{task_id}` | `GET /api/v2/tasks/{task_id}` | **Not path-only.** The id you poll with and the status values both changed — see API-018 |
| `POST /api/v1/memories/group` | *(none)* | **REMOVED — see API-012** |
| `POST /api/v1/memories/group/flush` | *(none)* | **REMOVED — see API-012** |
| `POST /api/v1/groups` | *(none)* | **REMOVED — see API-012** |
| `GET|PATCH /api/v1/groups/{group_id}` | *(none)* | **REMOVED — see API-012** |
| `POST /api/v1/senders` | *(none)* | **REMOVED — see API-013** |
| `GET|PATCH /api/v1/senders/{sender_id}` | *(none)* | **REMOVED — see API-013** |
| `GET|PUT /api/v1/settings` | *(none)* | **REMOVED — see API-014** |

### Search Patterns:
- `/api/v1/` in any string literal, constant, config file, `.http`/`.rest` file, Postman
  collection, OpenAPI client config, or environment variable
- `api/v1/memories` (note: plural) — the highest-signal single pattern
- Base-URL constants that append version, e.g. `BASE + "/api/v1"`

### Steps:
1. FIND every `/api/v1/` occurrence, including in non-source files (`.env`, YAML, JSON
   fixtures, docs, test recordings/VCR cassettes).
2. For each, look up the table above. Do NOT blanket-replace `v1` -> `v2`: three paths
   are removed entirely and `memories` becomes `memory`.
3. For removed paths, apply API-012/013/014 (flag, do not rewrite).
4. **If the version lives in a constant, do not bump it.** A typed client usually has
   something like `const apiVersion = "v1"` feeding six path roots. Changing it to `"v2"`
   silently repoints the three *removed* endpoints at `/api/v2/...`, where they 404 — which
   turns a documented, flaggable blocker into what looks like an outage. Split it instead:

   ```go
   const apiVersion       = "v2"   // migrated endpoints
   const legacyAPIVersion = "v1"   // EVEROS-MIGRATION: removed in v2, see API-012/013/014
   ```

   Pin the removed roots to the legacy constant so they fail loudly and stay findable.

---

## API-002: Authentication (unchanged)

### Change Type: NONE - Informational

Both versions use the same scheme and the same key:

```
Authorization: Bearer <api_key>
Content-Type: application/json
```

**No key re-issue and no auth code changes are required.** Verified live on prod
(2026-09-04): a single key completed a full v1 round trip and a full v2 round trip.

If a v2 call returns `401`, the key belongs to a different environment (keys are
environment-scoped: a dev/test key will 401 against prod). If it returns
`403 VERSION_NOT_ALLOWED`, the account is not v2-enabled yet.

---

## API-003: `add` request body - COMPLETE REWRITE

### Change Type: BREAKING - Body Rewrite

The owner identifier moves off the request and onto each message; `session_id` becomes
required; message timestamps change unit (see API-004).

**Before (v1):** `POST /api/v1/memories`
```json
{
  "user_id": "user-alice",
  "session_id": "session-1",
  "async_mode": false,
  "messages": [
    {"role": "user", "content": "I love hiking", "timestamp": 1757001600000}
  ]
}
```

**After (v2):** `POST /api/v2/memory/add`
```json
{
  "app_id": "default",
  "project_id": "default",
  "session_id": "session-1",
  "async_mode": false,
  "messages": [
    {
      "sender_id": "user-alice",
      "sender_name": "Alice",
      "role": "user",
      "content": "I love hiking",
      "timestamp": 1757001600000
    }
  ]
}
```

### Field Mapping:

| v1 | v2 | Notes |
|---|---|---|
| `user_id` (top level) | `messages[].sender_id` | **Moved onto every message.** This is the id that `get`/`search` later scope by. |
| `session_id` (optional) | `session_id` (**required**, 1–128 chars) | Now the unit extraction works on. A missing/empty value is a 422. |
| `messages[].role` | `messages[].role` | Unchanged: `user` \| `assistant` \| `tool` |
| `messages[].content` | `messages[].content` | Unchanged: string, or a list of content items for multimodal |
| `messages[].timestamp` | `messages[].timestamp` | **Unit enforced — see API-004** |
| *(v1 `sender_id` in group add)* | `messages[].sender_id` | Per-message sender is now the only way to attribute a turn |
| *(new)* | `messages[].sender_name` | Optional display name; does not affect scoping |
| *(new)* | `app_id` / `project_id` | See API-005 |
| `async_mode` | `async_mode` | Same flag, **different downstream behaviour — see API-011** |

### Constraints:
- `messages`: 1–500 items per call
- `session_id`: 1–128 characters
- An assistant turn carrying tool calls uses the OpenAI shape (`tool_calls`), followed by
  a `role: "tool"` message carrying `tool_call_id`.

### Where does `sender_id` come from?

`sender_id` is **required on every message** for a raw HTTP caller. v1 had a single
top-level `user_id` and no per-message sender, so there is usually **no agent id anywhere
in a v1 codebase to migrate from** — you have to introduce one.

- A **user** turn takes the user id that used to be the top-level `user_id`.
- An **assistant** turn takes the agent's own id. Introduce one (a stable string such as
  `"acme-assistant"`, ideally configurable) and flag it for the customer to confirm.
- This is not cosmetic. Per API-015, `sender_id` is what decides whether a write becomes
  agent memory, so stamping every turn with the human's id silently reclassifies the whole
  conversation.
- If you omit `sender_id`, the Python SDK defaults it to the **role string**, producing
  memories owned by a user literally called `"user"`. Raw callers should never rely on
  that: send it explicitly.

### Steps:
0. **If a message already carries `sender_id`, keep that value.** 0.4.x's personal add
   already accepted a per-message `sender_id`, so a v1 codebase may well have attributed
   assistant turns correctly. Overwriting them with the top-level `user_id` is silent,
   permanent, and lands in the extracted memory rather than in an error. Only fill in
   messages that lack one.
1. FIND the v1 add payload construction.
2. MOVE the top-level `user_id` into each message object as `sender_id`. If the code
   built messages in a loop, `sender_id` must be set per iteration — an assistant turn
   takes the agent's id, not the user's.
3. ENSURE `session_id` is always set and non-empty. If v1 code omitted it, generate one
   (a conversation/thread id is the natural choice) — do NOT hardcode a shared constant,
   because extraction boundaries are per-session.
4. APPLY API-004 to every `timestamp`.
5. ADD `app_id`/`project_id` only if the project needs non-default scoping (API-005).

---

## API-004: `timestamp` must be unix MILLISECONDS

### Change Type: BREAKING - Validation (hard failure)

**This is the highest-frequency migration break. It fails at runtime, not at compile
time, and it fails on every single write.**

v2 rejects a seconds-scale timestamp rather than silently rescaling it, because a batch
mixing the two scales would mis-order and mis-split sessions.

**Verified live on prod (2026-09-04)** — sending `"timestamp": 1757001600` returns:
```json
{
  "code": "InvalidParameter",
  "message": "The parameter `messages[0].timestamp` specified in the request are not valid: `timestamp` must be a unix millisecond timestamp (>= 1000000000000).",
  "param": "messages[0].timestamp",
  "type": "UnprocessableEntity",
  "status_code": 422
}
```

### Search Patterns (high value — scan for these even if nothing else changes):
- `time.time()` / `datetime.now().timestamp()` not followed by `* 1000` (Python)
- `Date.now() / 1000` or `Math.floor(Date.now()/1000)` (JS — the `/1000` is the bug)
- `time.Now().Unix()` — should be `.UnixMilli()` (Go)
- `System.currentTimeMillis() / 1000` (Java)
- `date +%s` (shell)
- Any integer timestamp literal in a fixture with 10 digits (seconds) rather than 13 (ms)

### Steps:
1. FIND every value that reaches `messages[].timestamp`.
2. If it is seconds, multiply by 1000 and cast to int.
3. Check test fixtures and seed data too — 10-digit literals are the giveaway.
4. If a timestamp is omitted entirely, most SDKs stamp "now" for you; raw HTTP callers
   must supply it (`timestamp` is a required field on `MessageItem`).

```python
# WRONG                          # RIGHT
"timestamp": int(time.time())    "timestamp": int(time.time() * 1000)
```

Per language:

| Language | Correct form |
|---|---|
| Python | `int(time.time() * 1000)` |
| JS / TS | `Date.now()` — already milliseconds; the bug is a `/ 1000` |
| Go | `time.Now().UnixMilli()` |
| Java | `System.currentTimeMillis()` |
| Shell | `$(( $(date +%s) * 1000 ))` |

**Shell needs the portable form.** `date +%s%3N` is GNU-only: BSD/macOS `date` has no `%N`
and emits the literal `3N`, producing a timestamp the server rejects. The portable form
costs sub-second precision, which is fine for live traffic but matters for a backfill —
see API-016.

### Search by the sink, not the source

Matching `time.time()` misses the common shapes:

```python
now = datetime.now(timezone.utc)      # any argument defeats a literal pattern
ts  = int(now.timestamp())            # and the two-line form defeats it entirely
```

Search for what flows **into** the field: every `"timestamp":` / `timestamp=` assignment,
plus `\.timestamp\(\)`, `time\.time\(\)`, `Date\.now\(\)`, `\.Unix\(\)`,
`date \+%s`, `/ *1000`, and any 10-digit integer literal in a fixture.

---

## API-005: New `app_id` / `project_id` scope

### Change Type: NEW - Concept with no v1 equivalent

v2 adds a two-part business-semantic partition to every memory call. Both default to
`"default"`, so a straight migration can ignore them — but the decision should be made
deliberately, not by default.

```json
{"app_id": "default", "project_id": "default", ...}
```

### Rules:
- **Reads must use the same `app_id`/`project_id` pair as the write.** A mismatched pair
  silently returns empty results — it is not an error.
- This is a **partition, not a security boundary.** The security boundary is the tenant
  resolved from the API key. Do not use `app_id` to isolate untrusted tenants.
- Applies to `add`, `get`, `search`, `delete`, and `edit`.

### Steps:
1. If the project is single-application, leave both at `"default"` and move on.
2. If the project serves multiple apps/environments/customers from one key, decide the
   mapping NOW and apply it consistently to every call site — retrofitting later means
   the old data is stranded under `default`.
3. FLAG this to the user as a design decision rather than silently defaulting, if the
   codebase shows signs of multi-tenancy (a tenant/org/workspace id threaded through
   the memory calls).

---

## API-006: `get` / `search` scoping — `filters` object -> top-level args

### Change Type: BREAKING - Body Rewrite

**Before (v1):** `POST /api/v1/memories/get`
```json
{"memory_type": "episodic_memory", "filters": {"user_id": "user-alice"}, "page": 1, "page_size": 20}
```
`POST /api/v1/memories/search`
```json
{"query": "outdoor hobbies", "filters": {"user_id": "user-alice"}, "top_k": 5}
```

**After (v2):** `POST /api/v2/memory/get`
```json
{"memory_type": "episode", "user_id": "user-alice", "page": 1, "page_size": 20}
```
`POST /api/v2/memory/search`
```json
{"query": "outdoor hobbies", "user_id": "user-alice", "method": "hybrid", "top_k": 5}
```

### Field Mapping:

| v1 | v2 | Notes |
|---|---|---|
| `filters.user_id` | `user_id` (top level) | Promoted out of the filters object |
| `filters.group_id` | *(none)* | **REMOVED — see API-012** |
| `filters.session_id` | *(not a get/search filter)* | Session scoping survives only on `delete` |
| `memory_type` | `memory_type` | **Values renamed — see API-007** |
| *(implicit)* | `agent_id` | New: read an agent's own memories |
| `top_k` | `top_k` | Default is now `-1` (engine decides); explicit values must be 1–100 |
| `method` | `method` | `keyword` \| `vector` \| `hybrid` (default) \| `agentic`. Not new in v2 — v1 offered the same four. |
| *(new)* | `min_score` | Honoured on the episode hybrid path only. `agentic` ignores it silently, so the pair is a no-op rather than an error. |

### Constraints:
- **Exactly one of `user_id` / `agent_id` is required** on both `get` and `search`.
  Passing neither, or both, is a 422.
- On `get`, owner and type must agree: a `user_id` owner may only request `episode` or
  `profile`; an `agent_id` owner may only request `agent_case` or `agent_skill`.
  Mismatched pairs are rejected with 422.
- `query` must be non-empty on `search`.

---

## API-007: `memory_type` value renames

### Change Type: BREAKING - Enum Rename

| v1 value | v2 value | Notes |
|---|---|---|
| `episodic_memory` | `episode` | |
| `profile` | `profile` | Unchanged |
| `agent_memory` | `agent_case` **or** `agent_skill` | **Split into two types** — pick per call site |
| `raw_message` | *(not retrievable)* | No longer a `get` type. Unextracted messages now surface only inside a `search` response as `unprocessed_messages` (API-008). |

### Search Patterns:
- `"episodic_memory"`, `'episodic_memory'` in any language
- `"agent_memory"` — every occurrence needs a human decision between case and skill
- `"raw_message"` — every occurrence needs rework, there is no drop-in replacement

### Steps:
1. REPLACE `episodic_memory` -> `episode`.
2. For `agent_memory`, read the surrounding code: retrieving a past trajectory is
   `agent_case`; retrieving a reusable procedure is `agent_skill`. If it is ambiguous,
   FLAG it rather than guessing.
3. For `raw_message`, FLAG with a comment. The caller must either accept
   `unprocessed_messages` from `search`, or keep its own copy of raw turns.

---

## API-008: Response envelope + field renames

### Change Type: BREAKING - Response Structure

**`request_id` moved to the top level** and the human-readable `message` field is gone.

**add** — before (v1) / after (v2):
```json
{"data": {"task_id": "0217...", "message_count": 4, "status": "accumulated", "message": "Messages accepted"}}
{"request_id": "0217...", "data": {"message_count": 4, "status": "extracted"}}
```

**flush** — before / after:
```json
{"data": {"request_id": "0217...", "status": "extracted", "message": "Flush completed"}}
{"request_id": "0217...", "data": {"status": "extracted"}}
```

**search** response fields:

| v1 field | v2 field | Notes |
|---|---|---|
| `episodes` | `episodes` | Unchanged |
| `profiles` | `profiles` | Unchanged |
| `raw_messages` | `unprocessed_messages` | Renamed |
| `agent_memory` (single, nullable) | `agent_cases` + `agent_skills` (two arrays) | Split |
| `query` (echo of the request) | *(none)* | Removed |
| `original_data` | *(none)* | Removed |

**get** response shape is unchanged: `episodes` / `profiles` / `agent_cases` /
`agent_skills` / `total_count` / `count`.

### Steps:
1. FIND response field access on add/flush results.
   - **flush:** `response["data"]["request_id"]` becomes `response["request_id"]`.
   - **add:** v1 carried `data.task_id`, not `data.request_id` (0.4.1's `AddResult` fields
     are `message`, `message_count`, `status`, `task_id`). v2 drops it and the id to poll
     with becomes the envelope's `request_id` — see **API-018**, which is where async
     callers should go.
2. FIND `raw_messages` -> `unprocessed_messages`.
3. FIND `agent_memory` access — it is now two arrays; a caller that read a single object
   needs restructuring, not a rename.
4. FIND any dependency on the `message` string (e.g. logging `"Messages accepted"`) and
   remove it.

---

## API-009: `delete` — status code, body, and scope semantics

### Change Type: BREAKING - Response + Semantics

**Before (v1):** returns `204 No Content` with an empty body.
**After (v2):** returns `200` with a body:
```json
{"request_id": "0217...", "data": {"filters": ["user_id", "session_id"], "count": 4}}
```

Code that checked `status == 204` will now see `200` and treat it as unexpected.

### Scope semantics (verified live on prod, 2026-09-04):

| v2 request | Effect |
|---|---|
| `{"user_id": "u"}` | Deletes the user's episodes **and profile** |
| `{"user_id": "u", "session_id": "s"}` | Deletes what that session produced; **the profile survives** (a profile is not session-derived) |
| `{"session_id": "s"}` | Allowed without an owner |

`user_id` and `agent_id` cannot be combined. At least one of
`user_id` / `agent_id` / `session_id` is required.

### REMOVED: single-memory delete by id

v1 accepted `{"memory_id": "<id>"}` to delete one memory cell. **v2 has no `memory_id`
mode** — `DeleteInput` declares `additionalProperties: false` and accepts only
`app_id` / `project_id` / `user_id` / `agent_id` / `session_id`. A v1 call that deleted
a single memory by id has no v2 equivalent; the nearest option is a session-scoped
delete, which is coarser. FLAG these call sites.

> **Note for cleanup scripts:** if v1 code relied on a session-scoped delete to fully
> remove a user, that assumption was already wrong on v1 and is still wrong on v2. Use a
> user-scoped delete to remove the profile.

### Steps:
1. FIND `204` checks on delete responses and change to `200`.
2. FIND callers that ignored the delete response and consider using `data.count`.
3. Re-check any "forget this user" / GDPR-style flow against the table above.

---

## API-010: Error response body shape

### Change Type: BREAKING - Error Contract

**Before (v1):**
```json
{"code": "HTTP_ERROR", "message": "Settings not initialized", "request_id": "0217...", "timestamp": "2026-09-04T19:16:42Z", "path": "/api/v1/settings"}
```

**After (v2):** there is **no single shape**. Three are in use, verified live on prod
(2026-09-14):

```json
// 400, 404, and 422 InvalidParameter — flat
{"code": "InvalidParameter", "message": "...", "param": "messages[0].timestamp",
 "type": "UnprocessableEntity", "status_code": 422}

// 422 from request-model validation — enveloped, with request_id
{"request_id": "unknown", "error": {"code": "invalid_argument", "message": "Value error, ..."}}

// 401 — enveloped, without request_id
{"error": {"code": "AuthenticationError", "message": "...", "param": "",
           "type": "Unauthorized", "status_code": 401}}
```

The contract declares `ErrorEnvelope` (`{request_id, error:{code, message}}`),
`HTTPValidationError` (`{detail: [...]}`) and `GatewayError`, whose own description admits
the shape "is not yet uniform across auth/quota/rate-limit paths — treat fields as
best-effort". `param` and `status_code` appear nowhere in the contract but do appear on the
wire.

**So: branch on the HTTP status code, and read the body defensively.**

```python
body = resp.json()
err  = body.get("error", body)          # handles both flat and enveloped
code = err.get("code")
msg  = err.get("message")
```

Also note two status codes that are easy to get wrong: a **missing required field returns
400**, not 422, and a **bad value returns 422**.

| v1 | v2 | Notes |
|---|---|---|
| `code` (`"HTTP_ERROR"`) | `code` (specific, e.g. `InvalidParameter`) | Values differ — code that matched on `"HTTP_ERROR"` will never match |
| `message` | `message` | Present in both |
| *(none)* | `param` | New: the offending field path |
| *(none)* | `type` | New: e.g. `UnprocessableEntity` |
| *(none)* | `status_code` | New: mirrors the HTTP status |
| `request_id` | *(in success bodies; not guaranteed here)* | Do not depend on it in error handling |
| `path`, `timestamp` | *(none)* | Removed |

### Steps:
1. FIND error handling that string-matches `"HTTP_ERROR"` and rewrite against the
   HTTP status code plus the new `code`/`type` values.
2. Branch on the **HTTP status**, never on a body field and never on `message` text:
   401 = bad or wrong-environment key, 403 = account not v2-enabled, 400 = missing field,
   422 = bad value, 429 = quota. Use the body only for the human-readable detail.
3. Unwrap defensively with `body.get("error", body)`. Code that reaches straight for
   `body["code"]` breaks on 401 and on validation errors; code that reaches straight for
   `body["error"]["code"]` breaks on 400, 404 and 422 InvalidParameter.

---

## API-011: Extraction timing — synchronous `add` now extracts

### Change Type: BEHAVIOURAL - Silent

Same flag name, different downstream result. **Verified live on prod (2026-09-04).**

| | v1 `async_mode: false` | v2 `async_mode: false` |
|---|---|---|
| `add` returns | `status: "accumulated"` | `status: "extracted"` |
| following `flush` returns | `status: "extracted"` | `status: "no_extraction"` |

The v2 sync path already ran extraction, so the subsequent `flush` correctly reports
that there was nothing left to do. **Code that asserts `flush` returned `"extracted"`,
or that treats `"no_extraction"` as a failure, will break** — even though the migration
otherwise succeeded and the memory is readable.

With `async_mode: true` (the default) the write is enqueued (`status: "queued"`,
HTTP 202) and an immediately following `flush` returns `"no_extraction"` because the
messages have not landed yet. Use `async_mode: false` for deterministic tests.

### Steps:
1. FIND assertions/branches on flush `status`.
2. Accept `"no_extraction"` as a non-error outcome, or drop the redundant `flush` after
   a synchronous `add` entirely.

---

## API-012: REMOVED — group memory (no v2 equivalent)

### Change Type: BREAKING - Removed, NO REPLACEMENT

**Do NOT rewrite these calls. FLAG them in place with a comment and stop.**

Removed with nothing to migrate to:
- `POST /api/v1/memories/group` — add multi-party group memory
- `POST /api/v1/memories/group/flush`
- `POST /api/v1/groups` — create group
- `GET|PATCH /api/v1/groups/{group_id}`
- `filters.group_id` on `get` / `search`
- `group_id` on `delete`

The v2 schema contains **no group concept whatsoever** (zero occurrences of `group` in
the v2 OpenAPI contract). v2 scopes memory by `user_id` or `agent_id` only.

v1 group memory produces a genuinely different artifact — an episode attributed to
multiple participants:
```json
{"group_id": "grp-1", "participants": ["bob", "alice"],
 "summary": "Alice suggested shipping a release on Friday. Bob replied that Friday works..."}
```
There is no way to produce that in v2 today.

### Steps:
1. FLAG every call site with a comment, e.g.:
   ```
   # EVEROS-MIGRATION: group memory has no v2 equivalent. This call cannot be migrated.
   # Options: (a) stay on v1 for this path, (b) model each participant as a separate
   # sender_id in one session and accept the loss of group-level aggregation.
   # Contact EverOS before choosing.
   ```
2. Do NOT delete the code and do NOT invent a replacement.

   **Flagging by format.** A comment is not always available:

   | Format | How to flag |
   |---|---|
   | Python, Go, TS, Java | A comment above the call site |
   | Shell | A comment **on its own line above** the command. A trailing comment swallows the rest of the line, and a comment placed after a `\` continuation silently splits one command into two. `bash -n` accepts both corruptions. |
   | JSON, Postman collections | No comment syntax exists. Put the reason in a `description` or metadata field, and **move the item into a separate file CI does not run**. Postman has no per-request disable, and `postman.setNextRequest(null)` does not prevent the request firing. |
   | VCR cassettes, recorded fixtures | Same: reason in a metadata field, move out of the executed set. |
3. Report the count of flagged group call sites prominently in the final summary — this
   is the finding that determines whether the migration can complete at all.

---

## API-013: REMOVED — sender registry (no v2 equivalent)

### Change Type: BREAKING - Removed, NO REPLACEMENT

- `POST /api/v1/senders` — register a sender with a display name
- `GET|PATCH /api/v1/senders/{sender_id}`

v2 has no sender registry. The closest thing is the optional per-message
`sender_name` field (API-003), which is **not** a stored registry: it is a display hint
attached to each message and it does not affect scoping.

### Steps:
1. If the registry was only used to attach display names, migrate by passing
   `sender_name` on each message — note this changes "register once" into "send every
   time", so the name must now be available at write time.
2. If the registry was read back (`GET /senders/{id}`) as a source of truth, FLAG it —
   there is nothing to read back from in v2.

---

## API-014: REMOVED — memory-space settings (no v2 equivalent)

### Change Type: BREAKING - Removed, NO REPLACEMENT

- `GET|PUT /api/v1/settings` — `llm_custom_setting` (per-stage model/provider overrides)
  and `timezone`

The v2 contract has no settings endpoint and no `timezone` or `llm_custom_setting`
field anywhere.

### Steps:
1. FLAG every call site.
2. **Ask EverOS whether existing v1 settings still apply to v2 processing for this
   account.** This is an open question, not a documented behaviour — an account that
   configured a non-UTC timezone or a custom extraction model may see different
   extraction results after migrating, with no way to reconfigure.

---

## API-015: Agent memory folded into the unified `add`

### Change Type: BREAKING - Endpoint Consolidation

v1 had a separate `POST /api/v1/memories/agent`. v2 has one `add` endpoint; whether a
write becomes agent memory is determined by the `sender_id` on each message and read
back via `agent_id`.

**Before (v1):**
```
POST /api/v1/memories/agent   {"user_id": "...", "messages": [...]}
POST /api/v1/memories/get     {"memory_type": "agent_memory", "filters": {"user_id": "..."}}
```

**After (v2):**
```
POST /api/v2/memory/add   {"session_id": "...", "messages": [{"sender_id": "<agent-id>", ...}]}
POST /api/v2/memory/get   {"agent_id": "<agent-id>", "memory_type": "agent_case"}
```

### Notes:
- Agent trajectories use the OpenAI tool-calling shape: an `assistant` message with
  `tool_calls`, then a `role: "tool"` message with `tool_call_id`.
- Retrieval lag: a distilled `agent_case` is readable via `get` within a few seconds
  while `search` may still return 0 hits (the vector index lags extraction). Prefer
  `get` for agent cases and skills.

---

## API-016: Data does not carry over (cutover planning)

### Change Type: OPERATIONAL - Not a code change

**v1 and v2 are separate stores under the same account.** Verified live on prod
(2026-09-04), in both directions:

- Data written via v1, read via `POST /api/v2/memory/get` for the same user id -> empty
- Data written via v2, read via `POST /api/v1/memories/get` **and**
  `/api/v1/memories/search` for the same user id -> empty (with `filters_applied`
  confirming the user id was passed)

Switching a running application from v1 to v2 means **its memory starts empty.** No code
change fixes this.

### Steps (report these to the user, do not attempt them automatically):
1. Decide a cutover strategy: hard cutover with an empty v2 store, dual-write during a
   transition window, or a backfill of historical conversations through `/api/v2/memory/add`.
2. If backfilling, note that historical messages need real historical timestamps in
   **milliseconds** (API-004), and that extraction is per-`session_id`, so the original
   conversation boundaries must be preserved to get comparable episodes. If the backfill is
   driven from shell, the portable `$(( $(date +%s) * 1000 ))` form loses sub-second
   precision — fine for live traffic, but it can reorder messages that arrived within the
   same second. Carry the original millisecond value through instead of re-deriving it.
3. Do not delete v1 data until v2 is verified in production.

---

## API-017: New in v2 (informational)

Do NOT auto-add these. Mention them in the summary only.

- `POST /api/v2/memory/edit` — bulk add/update/delete of individual profile items
- `POST /api/v2/memory/tag/bind` | `/tag/replace` | `/tag/unbind` — memory tagging
- `/api/v2/knowledge_bases/*` — knowledge bases, documents, categories, topics, tags,
  and KB-scoped search
- `GET /api/v2/tasks` and `GET /api/v2/tasks/stats` — task listing and aggregate stats
  (v1 had only per-task lookup)

---

## API-018: Async task polling

### Change Type: BREAKING - Silent for half of it

If you write with `async_mode: true` and then poll the task, two independent things changed
and neither is a path rename.

### (a) The add response no longer carries a task id

**Verified live on prod (2026-09-14).** An async add returns HTTP 202 and:

```json
{"data": {"message_count": 1, "status": "queued"}, "request_id": "0217894139161160..."}
```

`data` holds only `message_count` and `status`. **The id to poll with is the envelope's
top-level `request_id`.**

```
GET /api/v2/tasks/0217894139161160...
-> 200 {"data": {"id": "0217894139161160...", "status": "queued",
                 "task_type": "memory_add", "created_at": "..."}, "request_id": "..."}
```

The task endpoint echoes it back as `data.id`, so the envelope's `request_id` and the task's
`id` are the same value.

This one fails loudly: reading `task_id` off the add result raises `AttributeError` (Python) or
yields `undefined` (JS) on the first async write.

### (b) The status vocabulary changed, and this half fails silently

| v1 | v2 |
|---|---|
| `completed` | `success` |
| *(n/a)* | `queued`, `pending`, `processing` are all non-terminal |
| `failed` | `failed` |

The full v2 set, as reported by `GET /api/v2/tasks/stats`, is
`queued`, `pending`, `processing`, `success`, `failed`. A progression of
`queued -> processing -> success` was observed live (2026-09-14).

A leftover terminal check like:

```python
if status in ("completed", "failed", "error"):   # never true on v2
```

turns a finished task into an apparently-unfinished one, and the poll spins until its own
timeout. Nothing raises, and nothing logs.

> Treat only `success` and `failed` as terminal. A check that stops on `processing` or
> `pending` is the mirror-image bug: it reports a task done before it is.

### Steps:
1. FIND every read of a task id off an add response. The id now comes from the envelope's
   `request_id`, not from `data`.
2. FIND every status comparison against `"completed"` and change it to `"success"`.
3. Make sure the non-terminal set is `queued` / `pending` / `processing`, and that the loop
   keeps polling on all three.

### Search Patterns:
- `task_id` anywhere near an add call
- `tasks.retrieve(`, `/tasks/` in a URL
- the literal `"completed"` in a status comparison

---

## Applying the rules: order and hazards

There is no blanket search-and-replace table in this file. The one that used to be here was
order-dependent and mislabelled "safe": applying the `add` row before the others turns
`/api/v1/memories/get` into `/api/v2/memory/add/get`, and it contradicts API-001's own
instruction not to blanket-replace.

Work in this order:

1. **Record the blockers first.** Count and locate every `/api/v1/groups`, `/api/v1/senders`,
   `/api/v1/settings`, `memory_id` delete and `raw_message` **before** any path rewrite,
   while they are still findable.
2. **Types and shared constants** (typed clients): the type layer, then split the version
   constant per API-001 step 4.
3. **Endpoint paths**, one rule at a time, anchored. `/api/v1/memories` as the add endpoint
   must be anchored to end-of-token (`"`, `'`, end of line) or it eats the sub-paths.
4. **Request bodies** — API-003, API-004, API-005, API-006.
5. **Response handling** — API-008. Remember `data` stays on the wire.
6. **Errors** — API-010, branching on status.
7. **Task polling** — API-018.
8. **Recorded fixtures, cassettes and collections** last, so they match the code you just
   wrote.

Two substitutions are genuinely portable and context-free:

| Find | Replace |
|---|---|
| `"episodic_memory"` | `"episode"` |
| `raw_messages` (response field) | `unprocessed_messages` |

Everything else needs the rule.

**Flag only, never rewrite:** anything touching `group` (API-012), `/senders` (API-013),
`/settings` (API-014), `memory_id`-based delete (API-009), `"raw_message"` as a retrieval
type (API-007).
