# Migration Rules: everos-cloud 0.4.x (v1 API) -> 1.x (v2 API)

Package name is unchanged (`everos-cloud`), import path is unchanged (`everos_cloud`).
Everything else about the call surface changed: 1.x is a rewrite from a hand-written
httpx client onto an OpenAPI-generated client plus a thin `EverOS` facade.

**Read `../http/v1-to-v2.md` first.** It describes the wire-level changes (endpoints,
payloads, timestamps, removed capabilities) and is the semantic source of truth. This
file maps those changes onto Python SDK call sites.

> **Version naming.** The SDK versions are `0.4.x` -> `1.x`; the API versions they call
> are `v1` -> `v2`. This file is named `v1-to-v2.md` after the **API** version, matching
> the skill's rule-chaining convention. Tell the user "upgrade to everos-cloud 1.x
> (the v2 Memory API)" — never a bare "v2", which is ambiguous.

Verified against the published wheels for `0.4.1`, `1.0.0` and `1.1.0` (2026-09-04).
Target `>=1.1.0` unless the user asks otherwise.

## Preconditions

Same as `../http/v1-to-v2.md`: the account must be v2-enabled (`403 VERSION_NOT_ALLOWED`
otherwise), the API key does not change, v1 keeps working, and **existing memories do
not carry over** (API-016).

### PRE-001: Python 3.12 or newer is required — check this first

| Package | `requires_python` |
|---|---|
| `everos-cloud` 0.4.1 | `>=3.9` |
| `everos-cloud` 1.1.0 | **`>=3.12`** |

The interpreter floor moved three minor versions, and it is not mentioned in the public
migration guide. **Check it before touching a single call site.**

```
Grep pattern="requires-python|python_requires|python-version" glob="{pyproject.toml,setup.cfg,setup.py,.python-version,*.yml,*.yaml}"
```

If any declared target, CI matrix entry or `.python-version` is below 3.12, **STOP and
report it**. Migrating the code first produces the worst possible outcome: every call site
rewritten to the 1.x surface, then `pip install -U everos-cloud` quietly resolving back to
0.4.x (or `uv sync` hard-failing), leaving a repo that runs on neither version. Every
syntax check passes. Python 3.11 is supported until late 2027, so this is a live case, not
a corner one.

## Contents

- PRE-001: Python 3.12 or newer is required — check before anything else
- SDK-001: Package dependency (version constraint only)
- SDK-002: Client construction — `base_url` -> `host`, and **env vars are no longer read**
- SDK-003: Removed constructor options (`max_retries`, `http_client`, headers)
- SDK-004: REMOVED — `AsyncEverOS` (no async client in 1.x)
- SDK-005: Resource path `client.v1.memories.*` -> flat facade verbs
- SDK-006: `add()` — signature rewrite
- SDK-007: `flush()` — now keyed by `session_id`, not `user_id`
- SDK-008: `search()` — `filters` dict -> keyword args
- SDK-009: `get()` — `filters` dict -> keyword args, `memory_type` positional
- SDK-010: `delete()` — keyword-only, `memory_id` mode removed
- SDK-011: Return values — methods return `.data` directly
- SDK-012: Exception hierarchy collapsed
- SDK-013: Type imports — `everos_cloud.types.v1` is gone
- SDK-014: REMOVED — `groups`, `senders`, `settings` resources
- SDK-015: Low-level clients and the 1.1.0 surface (informational)
- SDK-017: `object.sign` -> `presign` (signature + error contract)
- SDK-018: Test doubles, fakes and fixtures
- SDK-016: Task polling — the task id moved off the add response onto the envelope
- Applying the rules: order and hazards

---

## SDK-001: Package dependency

### Change Type: BREAKING - Version Constraint

The package name does not change. Only the constraint does.

**Before (0.4.x):**
```
# pyproject.toml
dependencies = ["everos-cloud>=0.4.1"]
# requirements.txt
everos-cloud>=0.4.1
everos-cloud==0.4.1
everos-cloud<1          # a deliberate pin to stay on the v1 client
```

**After (1.x):**
```
dependencies = ["everos-cloud>=1.1.0"]
everos-cloud>=1.1.0
```

### Search Patterns:
- `everos-cloud` in pyproject.toml, requirements*.txt, setup.py, setup.cfg, Pipfile,
  poetry.lock / uv.lock (regenerate locks rather than hand-editing)
- **`everos-cloud<1`** — an explicit "stay on 0.4.x" pin; removing it is the point of
  this migration, but confirm with the user that it was not pinned for another reason

### Steps:
1. Update the constraint to `>=1.1.0`.
2. Do NOT run the install. Tell the user to run `pip install -U everos-cloud` or
   `uv sync` when they are ready.

---

## SDK-002: Client construction — `base_url` -> `host`, env vars no longer read

### Change Type: BREAKING - Signature + **SILENT BEHAVIOUR CHANGE**

**This rule contains the most dangerous change in the whole migration. Apply it even if
the client construction line otherwise looks fine.**

**Before (0.4.x):**
```python
from everos_cloud import EverOS

client = EverOS()                                   # worked: api_key read from env
client = EverOS(api_key=os.environ["EVEROS_API_KEY"],
                base_url=os.environ.get("EVER_OS_BASE_URL"))
```

**After (1.x):**
```python
from everos_cloud import EverOS

client = EverOS(api_key=os.environ["EVEROS_API_KEY"])          # api_key is REQUIRED
client = EverOS(api_key=os.environ["EVEROS_API_KEY"],
                host=os.environ.get("EVER_OS_BASE_URL"))       # base_url -> host
```

### The two traps:

**1. `api_key` is now required.** In 0.4.x it defaulted to `None` and the client read
`EVEROS_API_KEY` from the environment. In 1.x the signature is
`EverOS(api_key: str, *, host=None, app_id="default", project_id="default", timeout=...)`
— `api_key` is a required positional parameter and **nothing reads the environment**.
`EverOS()` raises `TypeError`. This fails loudly, so it is the safe one.

> The official migration guide states "still reads `EVEROS_API_KEY` if omitted".
> **That is incorrect** — verified against the published 1.0.0 and 1.1.0 wheels, which
> contain no `os.environ` or `getenv` reference anywhere in `client.py`.

**2. `EVER_OS_BASE_URL` is no longer read either — and this one fails SILENTLY.**
0.4.x picked the base URL up from the environment automatically. 1.x does not: if the
env var is set but nothing is passed to `host=`, the client falls back to the default
production host. Code that pointed at a dev or test gateway via the environment will
**silently start reading and writing production data** after the upgrade.

### Steps:
1. FIND every `EverOS(` construction, including in tests, fixtures, and conftest files.
2. RENAME `base_url=` to `host=`.
3. If `api_key` was omitted, add `api_key=os.environ["EVEROS_API_KEY"]` explicitly.
4. **Rewrite, do not merely flag.** This is the one place where flagging *creates* the
   failure it warns about: a comment above an unchanged `EverOS(...)` leaves CI, staging
   and every container pointed at production. Wherever `EVER_OS_BASE_URL` is set anywhere
   in the repo, write `host=os.environ.get("EVER_OS_BASE_URL")` into **every** `EverOS(`
   construction, then flag it for review. Flag the review, not the bug.
5. **Search the whole repo for `EVER_OS_BASE_URL`** — including `.env` files,
   docker-compose, CI configs, Dockerfiles and shell scripts. Match **file names only**:
   those files usually hold `EVEROS_API_KEY` and its live value on a neighbouring line, and
   you only need to know which files reference the variable, never what any of them are set
   to. Do not open them to read values and do not quote a matched line in the report. If the
   variable is set anywhere and is not explicitly passed to `host=`, FLAG it loudly:
   ```python
   # EVEROS-MIGRATION: 1.x no longer reads EVER_OS_BASE_URL from the environment.
   # host= is now passed explicitly below — confirm it points where you intend.
   ```
6. Consider adding `app_id=` / `project_id=` here — they are client-level defaults that
   every call inherits, which is cleaner than passing them per call (see http API-005).

---

## SDK-003: Removed constructor options

### Change Type: BREAKING - Removed, NO REPLACEMENT

0.4.x accepted `max_retries`, `default_headers`, `default_query`, `http_client`,
and rich `timeout` objects (`httpx.Timeout`). 1.x accepts only:

```python
EverOS(api_key, *, host=None, app_id="default", project_id="default", timeout=<float seconds>)
```

| 0.4.x option | 1.x | Notes |
|---|---|---|
| `max_retries=2` | *(none)* | **No retry layer.** Retries must be implemented by the caller. |
| `http_client=httpx.Client(...)` | *(none)* | No custom transport injection (proxies, mTLS, instrumentation) |
| `default_headers=` / `default_query=` | *(none)* | No per-client header injection |
| `timeout=httpx.Timeout(...)` | `timeout=<float>` | Seconds only, applied to every request |

### Also removed: `EVER_OS_CUSTOM_HEADERS`

0.4.x read this environment variable and merged it into `default_headers`
(`everos_cloud/_client.py:88`). 1.x reads no environment at all and has no
`default_headers` parameter, so a deployment injecting a routing or tenant header through
it loses that header silently. Grep for the variable name alongside the other two.

### Also changed: the HTTP transport

| | 0.4.x | 1.x |
|---|---|---|
| Transport | `httpx` | `urllib3` |

Nothing in the call surface exposes this, but test suites do. `respx`,
`httpx.MockTransport` and `httpx_mock` stop intercepting after the upgrade, so tests either
hit the network or fail in a way that looks unrelated to the migration. httpx-specific
proxy, certificate and `trust_env` configuration stops applying, and any instrumentation
hooked into httpx goes dark. FLAG all of these.

### Steps:
1. **Delete the removed keyword arguments; do not merely flag them.** `EverOS.__init__` is
   `(api_key, *, host, app_id, project_id, timeout)`, so leaving `max_retries=` or
   `http_client=` in place is a hard `TypeError` and the client never constructs. Remove
   the argument, then flag the behaviour that was lost.
2. Retries in particular are a silent reliability regression — 0.4.x retried twice by
   default, 1.x does not retry at all.
3. If the code relied on `max_retries`, suggest wrapping calls in the user's own retry
   (e.g. `tenacity`), and note that `EverOSAPIError` carries `.status` for deciding
   what is retryable (429 / 5xx).

---

## SDK-004: REMOVED — `AsyncEverOS`

### Change Type: BREAKING - Removed, NO REPLACEMENT

0.4.x exported `AsyncEverOS` (plus `AsyncClient`, `AsyncStream`, `AsyncAPIResponse`,
`DefaultAsyncHttpxClient`, `DefaultAioHttpClient`). **1.x has no async client at all** —
the facade is synchronous only.

### Search Patterns:
- `AsyncEverOS`, `AsyncClient`, `await client.`, `async with EverOS`
- `AsyncStream`, `AsyncAPIResponse`, `DefaultAsyncHttpxClient`, `DefaultAioHttpClient`

### Steps:
0. **Remove the module-level import first.** "Flag, do not rewrite" applies to the *call*,
   not to an `import` of a symbol that no longer exists. A leftover
   `from everos_cloud import AsyncEverOS` raises `ImportError` at import time and takes down
   the **whole module**, including the functions that migrated cleanly. Move the import into
   the function body so only the flagged path fails:

   ```python
   def legacy_async_path():
       from everos_cloud import AsyncEverOS   # EVEROS-MIGRATION: removed in 1.x, see below
       ...
   ```

   `python -m py_compile` does not catch this. `python -c "import <module>"` does.
1. FLAG every async call site — do NOT rewrite them into blocking calls silently, since
   that would block an event loop:
   ```python
   # EVEROS-MIGRATION: everos-cloud 1.x has no async client (AsyncEverOS was removed).
   # Options: (a) run the sync client in a thread executor
   #          (asyncio.to_thread(client.add, ...)), (b) call /api/v2/memory/* directly
   #          with your own async HTTP client, (c) stay on 0.4.x for this path.
   ```
2. Report the count of async call sites prominently — for an async codebase this is a
   blocking finding, not a cosmetic one.

---

## SDK-005: Resource path -> flat facade verbs

### Change Type: BREAKING - Method Path

**Before (0.4.x):** `client.v1.memories.add(...)`, `client.v1.settings.retrieve()`
**After (1.x):** `client.add(...)` — there is no `.v1`, and no `.memories` namespace.

The nine verbs frozen at 1.0.0 are bare: `add`, `search`, `get`, `flush`, `edit`,
`delete` (memory), `presign`, `upload` (storage), `close`.

> **"Unprefixed means memory" is false** — `presign` and `upload` are storage
> operations. Anything added after 1.0.0 is `<resource>_<verb>` (see SDK-015).

### Search Patterns:
- `client.v1.` — the single highest-signal pattern for a 0.4.x codebase
- `.v1.memories.`, `.v1.settings.`, `.v1.senders.`, `.v1.groups.`, `.v1.tasks.`

### Note on version detection:
1.x code has **no `client.vN.` prefix at all**. Do not try to detect the installed
version from a `client.vN.` pattern — for 1.x, detect on the dependency constraint
(`everos-cloud>=1`) plus bare facade verbs.

### Helpful runtime behaviour:
1.1.0's facade implements `__getattr__` so that calling a *generated* method name on the
facade raises an `AttributeError` naming both the facade equivalent and the low-level
location. If the user hits one of those messages after migrating, it is a hint, not a bug.

---

## SDK-006: `add()` — signature rewrite

### Change Type: BREAKING - Signature Rewrite

Implements http API-003 and API-004. See those rules for the wire semantics.

**Before (0.4.x):**
```python
response = client.v1.memories.add(
    user_id="user-alice",
    session_id="session-1",
    messages=[{
        "role": "user",
        "content": "I love hiking",
        "timestamp": int(time.time()),        # seconds — see API-004
        "sender_id": "user-alice",
    }],
    async_mode=True,
)
```

**After (1.x):**
```python
result = client.add(
    session_id="session-1",                    # now the first positional arg, REQUIRED
    messages=[{
        "sender_id": "user-alice",             # owner lives here now
        "role": "user",
        "content": "I love hiking",
        "timestamp": int(time.time() * 1000),  # unix MILLISECONDS
    }],
    async_mode=True,            # keep the caller's existing value; see the note below
)
# result is AddData: result.message_count, result.status
```

> **Do not change `async_mode` while migrating.** The flag means the same thing in both
> versions. Flipping a fire-and-forget write to synchronous moves extraction inline and
> changes request latency, and it orphans any downstream task poll. If the caller polls
> the task afterwards, see SDK-016 — the facade cannot reach the task id at all.

Signature: `add(session_id, messages, *, mode=None, async_mode=None, app_id=None, project_id=None)`

### Field Mapping:

| 0.4.x | 1.x | Notes |
|---|---|---|
| `user_id=` (top level) | `messages[].sender_id` | Moved onto each message |
| `session_id=` (optional) | `session_id` (**required**, first positional) | 1–128 chars |
| `messages[].timestamp` seconds | milliseconds | **Hard 422 — see API-004** |
| `async_mode=` | `async_mode=` | Same flag, different flush behaviour — see SDK-007 |
| *(new)* | `app_id=` / `project_id=` | Usually set once on the client instead |

### SDK ergonomic defaults (1.x only — know these before "fixing" code):
- A message with no `timestamp` is stamped with **now**. Good for live traffic, **wrong
  for backfill** — historical messages must carry their real timestamps.
- A message with no `sender_id` defaults to its **`role`** string. That silently
  produces memories owned by a user literally called `"user"`. When migrating a loop
  that built messages without an explicit sender, set `sender_id` explicitly.

---

## SDK-007: `flush()` — now keyed by `session_id`

### Change Type: BREAKING - Signature + Semantics

**Before (0.4.x):** `client.v1.memories.flush(user_id="user-alice")`
**After (1.x):** `client.flush("session-1")`

Signature: `flush(session_id, *, app_id=None, project_id=None)`

The unit of extraction moved from the user to the session. A codebase that flushed once
per user after several sessions must now flush per session.

### Behavioural change (http API-011):
After `add(..., async_mode=False)`, v2 has **already extracted**, so the following
`flush` returns `status="no_extraction"` — not `"extracted"`. Code asserting
`"extracted"` will fail even though the migration worked.

### Steps:
1. REWRITE `flush(user_id=...)` to `flush(<session_id>)`. If the session id is not in
   scope at the call site, FLAG it — this needs the caller's own restructuring.
2. FIND assertions on flush status and accept `"no_extraction"`, or drop the redundant
   flush after a synchronous add.

---

## SDK-008: `search()` — `filters` dict -> keyword args

### Change Type: BREAKING - Signature Rewrite

**Before (0.4.x):**
```python
response = client.v1.memories.search(
    filters={"user_id": "user-alice", "group_id": "grp-1"},
    query="outdoor hobbies",
    method="vector",
    top_k=5,
)
episodes = response.data.episodes
```

**After (1.x):**
```python
result = client.search(
    "outdoor hobbies",          # query is the first positional arg
    user_id="user-alice",       # exactly one of user_id / agent_id is REQUIRED
    method="vector",            # keyword | vector | hybrid (default) | agentic
    top_k=5,
)
episodes = result.episodes      # already unwrapped — see SDK-011
```

Signature: `search(query, *, method=None, top_k=None, user_id=None, agent_id=None,
include_profile=None, min_score=None, radius=None, enable_llm_rerank=None,
filters=None, app_id=None, project_id=None)`

### Field Mapping:

| 0.4.x | 1.x | Notes |
|---|---|---|
| `filters={"user_id": x}` | `user_id=x` | Promoted to a keyword arg |
| `filters={"group_id": x}` | *(none)* | **REMOVED — see http API-012, FLAG** |
| `query=` | first positional | Must be non-empty |
| `top_k=` | `top_k=` | Default `-1` (engine decides); explicit values 1–100 |
| `memory_types=[...]` | *(none)* | **REMOVED.** A search can no longer be restricted to a subset of memory types. The response still separates them into `episodes` / `profiles` / `agent_cases` / `agent_skills`, so the filtering moves to the caller. |
| `include_original_data=` | *(none)* | **REMOVED**, along with the `original_data` field it populated |
| *(new)* | `agent_id=`, `include_profile=`, `min_score=`, `radius=`, `enable_llm_rerank=` | See the caveat on `min_score` below |

> **`min_score` is honoured on the episode hybrid path only.** `method="agentic"` ignores it
> silently, so passing the two together is a no-op rather than an error. If you are adopting
> `min_score` as part of this migration, filter the returned `score` values yourself on the
> agentic path.

> `memory_types=[...]` is where an `agent_memory` or `raw_message` value actually lives on
> 0.4.x, not on `get` (SDK-009). `agent_memory` becomes a choice between `agent_case` and
> `agent_skill` that only a human can make; `raw_message` has no replacement, and what used
> to match it now arrives as `unprocessed_messages` in the response.

> A `filters=` parameter still exists on 1.x `search`/`get`, but it is a **passthrough
> for v2-native filters, not the v1 scoping dict**. Do NOT migrate
> `filters={"user_id": ...}` by leaving it as-is — the user id must move to `user_id=`.

### Response field renames (http API-008):
`raw_messages` -> `unprocessed_messages`; `agent_memory` -> `agent_cases` + `agent_skills`;
`query` and `original_data` removed.

---

## SDK-009: `get()` — `filters` dict -> keyword args

### Change Type: BREAKING - Signature Rewrite

**Before (0.4.x):**
```python
response = client.v1.memories.get(
    filters={"user_id": "user-alice"},
    memory_type="episodic_memory",
    page=1, page_size=20,
)
for ep in response.data.episodes: ...
```

**After (1.x):**
```python
result = client.get(
    "episode",                  # memory_type is the first positional arg
    user_id="user-alice",
    page=1, page_size=20,
)
for ep in result.episodes: ...
```

Signature: `get(memory_type, *, user_id=None, agent_id=None, page=None, page_size=None,
sort_by=None, sort_order=None, filters=None, app_id=None, project_id=None)`

### Field Mapping:

| 0.4.x | 1.x | Notes |
|---|---|---|
| `memory_type="episodic_memory"` | `"episode"` (positional) | See http API-007 |
| `rank_by=` / `rank_order=` | `sort_by=` / `sort_order=` | Renamed **and narrowed.** 0.4.x `rank_by` was a free-form `str`; v2 `sort_by` is `enum["timestamp", "updated_at"]` and `GetInput` is `additionalProperties: false`. Any other value is a runtime 422. |
| `filters={"user_id": x}` | `user_id=x` | |
| `filters={"group_id": x}` | *(none)* | **REMOVED — FLAG** |

### Constraint:
Owner and type must agree — `user_id` may only ask for `episode`/`profile`; `agent_id`
may only ask for `agent_case`/`agent_skill`. A mismatch is a 422 at runtime, not a
syntax error.

### `agent_memory` and `raw_message` are NOT `get` values — look on `search`

0.4.x's `get(memory_type=...)` is typed
`Literal['episodic_memory', 'profile', 'agent_case', 'agent_skill']`, so it never accepted
`agent_memory` or `raw_message`. Both appear only in `search(memory_types=[...])`
(see SDK-008). Verified by introspecting 0.4.1.

Aim the decision at the right call site: it is the `search` call that has to choose between
`agent_case` and `agent_skill`, and the `search` call that loses `raw_message`.

---

## SDK-010: `delete()` — keyword-only, `memory_id` mode removed

### Change Type: BREAKING - Signature + Removed Mode

**Before (0.4.x):**
```python
client.v1.memories.delete(memory_id="6a9b...")             # mode 1: single delete
client.v1.memories.delete(user_id="u", group_id="g")       # mode 2: batch by filter
```

**After (1.x):**
```python
result = client.delete(user_id="u", session_id="s")
# result is DeleteData: result.count, result.filters
```

Signature: `delete(*, user_id=None, agent_id=None, session_id=None, app_id=None, project_id=None)`

| 0.4.x | 1.x | Notes |
|---|---|---|
| `memory_id=` | *(none)* | **REMOVED — no single-memory delete. FLAG.** |
| `group_id=` | *(none)* | **REMOVED — see http API-012. FLAG.** |
| `sender_id=` | *(none)* | **REMOVED. FLAG.** |
| `user_id=` / `session_id=` | same | Unchanged. 0.4.x's `delete` was already keyword-only. |
| returns `None` (204) | returns `DeleteData` | See SDK-011 and http API-009 |

### Semantics to re-check (http API-009):
`delete(user_id=...)` removes episodes **and** the profile;
`delete(user_id=..., session_id=...)` leaves the profile in place. Re-verify any
"forget this user" flow against that.

---

## SDK-011: Return values — methods return `.data` directly

### Change Type: BREAKING - Return Type

0.4.x returned the full response envelope; 1.x facade methods return the response
**`.data` payload** already unwrapped.

```python
# 0.4.x
response = client.v1.memories.get(filters={"user_id": u}, memory_type="episodic_memory")
episodes = response.data.episodes
total    = response.data.total_count

# 1.x
result   = client.get("episode", user_id=u)
episodes = result.episodes
total    = result.total_count
```

### Search Patterns:
- `.data.` immediately after an everos call result — one `.data` level must be dropped
- `response.data is None` guards — no longer meaningful
- `response.request_id` — `request_id` lives on the envelope, which the facade discards.
  If the caller logs it, use the low-level client (`client.memory.*`) for that call.

### Steps:
1. REMOVE exactly one `.data` level from every result access.
2. Do NOT remove `.data` from things that are genuinely nested, e.g. a profile item's
   own `profile_data`.
3. FLAG any use of `request_id` from a facade result.

---

## SDK-012: Exception hierarchy collapsed

### Change Type: BREAKING - Exception Classes

0.4.x shipped an OpenAI-style hierarchy. 1.x collapses it to three classes.

**Before (0.4.x):** `EverOSError` -> `APIError` -> `APIStatusError` ->
`BadRequestError`, `AuthenticationError`, `PermissionDeniedError`, `NotFoundError`,
`ConflictError`, `UnprocessableEntityError`, `RateLimitError`, `InternalServerError`;
plus `APIConnectionError`, `APITimeoutError`, `APIResponseValidationError`.

**After (1.x):** `EverOSError` -> `EverOSAPIError` (HTTP errors, carries `.status` and
`.body`) and `EverOSStorageError` (upload/presign failures).

```python
# 0.4.x
from everos_cloud import RateLimitError, NotFoundError
try:
    client.v1.memories.search(filters={"user_id": u}, query="x")
except RateLimitError:
    backoff()
except NotFoundError:
    ...

# 1.x
from everos_cloud import EverOSAPIError
try:
    client.search("x", user_id=u)
except EverOSAPIError as e:
    if e.status == 429:
        backoff()
    elif e.status == 403:
        ...   # account not enabled for v2
```

### Steps:
1. REPLACE every granular exception class with `EverOSAPIError` + a `.status` check.
   The status mapping: 400 `BadRequestError`, 401 `AuthenticationError`,
   403 `PermissionDeniedError`, 404 `NotFoundError`, 409 `ConflictError`,
   422 `UnprocessableEntityError`, 429 `RateLimitError`, 5xx `InternalServerError`.
2. `APIConnectionError` / `APITimeoutError` have **no 1.x equivalent**. Transport failures
   surface as `urllib3` exceptions, which are not `EverOSError`s: a refused connection raises
   `urllib3.exceptions.MaxRetryError`, a timeout `urllib3.exceptions.ReadTimeoutError`, and
   both derive from `urllib3.exceptions.HTTPError` (verified on 1.1.0 against a closed port).
   **Do not delete the handler.** Rewrite it to `except urllib3.exceptions.HTTPError` (import
   `urllib3`; it is already a dependency of 1.x) and flag it, so the caller keeps the
   behaviour it had. Deleting the clause turns a swallowed outage into an uncaught exception,
   and that is a behaviour change the report must name.
3. `except EverOSError` keeps working (it is still the base class) — leave those alone.
4. **A low-level `client.memory.*` / `client.storage.*` call raises `ApiException`, not
   `EverOSAPIError`.** Only the facade's `_call` wrapper performs that translation, and
   `issubclass(ApiException, EverOSError)` is `False`. This matters because SDK-016 sends
   async pollers to the low-level client: apply both rules literally and the handler
   SDK-012 just rewrote becomes dead code, with no error raised at any point. The same call
   also bypasses the client's `timeout`, which `_call` supplies via `_request_timeout`.

   ```python
   from everos_cloud import EverOSAPIError
   from everos_cloud.exceptions import ApiException

   try:
       envelope = client.memory.add_memory(payload, _request_timeout=60)
   except (EverOSAPIError, ApiException) as e:
       ...   # ApiException also carries .status
   ```
5. **`EverOSAPIError` only covers errors the gateway returned.** 1.x validates the request
   body with pydantic *before* anything is sent, and those failures raise
   `pydantic_core.ValidationError`, which derives from `ValueError` and is **not** an
   `EverOSError` subclass. 0.4.x sent the same input to the server and surfaced it as a
   `BadRequestError`, so a caller that caught the SDK's exception and turned it into its own
   4xx now lets the exception escape instead. Where the caller passes user-supplied input
   straight into a call, widen the catch:

   ```python
   except (EverOSAPIError, ValueError) as e:
       ...
   ```

---

## SDK-013: Type imports — `everos_cloud.types.v1` is gone

### Change Type: BREAKING - Removed Module

```python
# 0.4.x
from everos_cloud.types.v1 import (
    AddResponse, GetMemoriesResponse, SearchMemoriesResponse, SettingsAPIResponse,
)
```

The `everos_cloud.types.v1` module does not exist in 1.x. Generated pydantic models live
under `everos_cloud.models.*` and the facade returns the `*Data` payload types.

### Steps:
1. REMOVE `from everos_cloud.types.v1 import ...` lines. This module does not exist in 1.x,
   so a leftover import raises `ImportError` at import time and takes the whole module with
   it, not just the annotated function. Same rule as SDK-004 step 0.
2. If the names were only used as type annotations, the simplest correct migration is to
   drop the annotations or use the model names from `everos_cloud.models`; do not guess
   at names — check the installed package.
3. `SettingsAPIResponse` and any group/sender types have no equivalent at all (SDK-014).

---

## SDK-014: REMOVED — `groups`, `senders`, `settings` resources

### Change Type: BREAKING - Removed, NO REPLACEMENT

**Do NOT rewrite. FLAG in place.** See http API-012, API-013, API-014 for the full
explanation and the wording to use.

| 0.4.x call | 1.x |
|---|---|
| `client.v1.memories.group.add(...)` | *(none)* |
| `client.v1.memories.group.flush(...)` | *(none)* |
| `client.v1.groups.create(...)` / `.retrieve(...)` / `.patch(...)` | *(none)* |
| `client.v1.senders.create(...)` / `.retrieve(...)` / `.patch(...)` | *(none)* — partial: per-message `sender_name` |
| `client.v1.settings.retrieve()` / `.update(...)` | *(none)* |
| `filters={"group_id": ...}` anywhere | *(none)* |

> **Method names matter here.** On 0.4.x, `groups` and `senders` expose
> `create` / `retrieve` / **`patch`** — neither has an `update`. Only `settings` has
> `.update(`. A search pattern built around `groups.update` or `senders.update` matches
> nothing and the call sites are silently missed. Verified by introspecting 0.4.1.

### Steps:
1. FLAG each call site with the reason and the options (see http API-012 step 1).
2. **Count them and surface the count at the top of the final report.** If this count is
   greater than zero, the migration cannot be completed by this tool and the user needs
   to talk to EverOS before proceeding.

---

## SDK-015: Low-level clients and the 1.1.0 surface (informational)

Do NOT auto-add these. Mention in the summary only.

- Low-level generated clients, returning the full envelope and raising `ApiException`:
  `client.memory`, `client.storage`, `client.knowledge`, `client.tasks`.
  Use them when the facade omits something (e.g. reading `request_id`).
- 1.0.0 froze nine bare verbs (SDK-005). Everything added since is `<resource>_<verb>`:
  - knowledge bases: `kb_create`, `kb_get`, `kb_list`, `kb_update`, `kb_delete`, `kb_search`
  - documents: `doc_ingest`, `doc_get`, `doc_list`, `doc_update`, `doc_delete`
  - tags: `tag_bind`, `tag_replace`, `tag_unbind`
  - tasks: `task_get`, `task_list`, `task_wait`
- `edit(user_id, operations)` — bulk profile item add/update/delete, new in v2.
- The client supports the context-manager protocol (`with EverOS(...) as client:`) and
  `close()` releases pooled connections.

---

## SDK-016: Task polling

### Change Type: BREAKING - Half of it silent

Implements http API-018. Applies to any caller that passes `async_mode=True` and then follows
the task. **This is the defect most likely to survive the migration and break at runtime**,
because SDK-011 ("drop one `.data` level") rewrites it into valid Python that raises.

**Before (0.4.x):**
```python
response = client.v1.memories.add(
    user_id=u, session_id=s, messages=msgs, async_mode=True,
)
task = client.v1.tasks.retrieve(response.data.task_id)
if task.data.status in ("success", "failed"):
    ...
```

**After (1.x):**
```python
from everos_cloud.models.add_input import AddInput
from everos_cloud.models.message_item import MessageItem
from everos_cloud.models.content import Content

# The facade returns .data, which has no task id. An async caller that follows its
# task needs the envelope, so it goes through the generated client.
envelope = client.memory.add_memory(AddInput(
    app_id="default", project_id="default",
    session_id=s, async_mode=True,
    messages=[
        MessageItem(
            sender_id=u, role=m["role"], timestamp=m["timestamp"],
            content=Content(m["content"]),     # not coerced for you here — see below
        )
        for m in msgs
    ],
))

task = client.task_get(envelope.request_id)
if task.status in ("success", "failed"):
    ...

# or let the SDK do the loop:
task = client.task_wait(envelope.request_id, timeout=180, interval=3)
```

### Field Mapping:

| 0.4.x | 1.x | Notes |
|---|---|---|
| `response.data.task_id` | `envelope.request_id` | **The add response carries no task id.** `AddData` has only `message_count` and `status`. |
| `client.v1.tasks.retrieve(id)` | `client.task_get(id)` | Returns an unwrapped `TaskItem`: `id`, `status`, `task_type`, `created_at`, `finished_at`, `error` |
| *(hand-rolled poll loop)* | `client.task_wait(id, ...)` | `timeout` / `interval` / `max_interval` / `raise_on_failure`, with backoff |
| `status` values | unchanged for SDK callers | 0.4.x already declared `Literal["processing", "success", "failed"]` — see the note below |

### Two traps

**1. `client.add()` cannot be used for this at all.** The facade returns the response `.data`
and discards the envelope, so `request_id` is unreachable through it. An async caller that
polls must use `client.memory.add_memory(...)`. SDK-011 says the facade drops the envelope;
this is the case where that actually costs you something.

**2. The status vocabulary is mostly unchanged — for SDK callers.** 0.4.x already types
`TaskStatusResult.status` as `Literal["processing", "success", "failed"]`, so a codebase
written against the SDK types is already comparing against `"success"`. The string
`"completed"` does not appear anywhere in the 0.4.1 wheel. v2 adds `queued` and `pending`
as further non-terminal states; the terminal pair is unchanged.

> No API generation ever returned `"completed"`; the v1 contract enumerates
> `processing | success | failed`. Do not go hunting for it in SDK code or anywhere else.

What to check instead: that the non-terminal set covers `queued`, `pending` **and**
`processing`, and that only `success` and `failed` stop the loop. A check that treats
`processing` as terminal reports a task finished before it is.

### Note on the low-level client

The facade's `add()` coerces a plain string into `Content` for you (`_to_message` does it).
The generated client does **not**: `MessageItem.content` is typed `Content`, so passing a bare
`str` makes pydantic reject the call before it reaches the network. Build `MessageItem` and
`Content` explicitly, as above.

### Search Patterns:
- `.task_id` anywhere near an add call
- `tasks.retrieve(`
- a loop that stops on anything other than `"processing"` (it now stops on `"queued"`)
- `async_mode=True` — every one of these call sites deserves a look

---

## SDK-017: `object.sign` -> `presign`

### Change Type: BREAKING - Signature + Error Contract

API-001 lists `/api/v1/object/sign` -> `/api/v2/object/sign` as a path-only change. At the
SDK level it is not.

**Before (0.4.x):**
```python
resp = client.v1.object.sign(object_list=[{"object_name": "a.png", "method": "PUT"}])
if resp.status != 0:
    handle(resp.error)
urls = resp.result
```

**After (1.x):**
```python
urls = client.presign([{"object_name": "a.png", "method": "PUT"}])   # positional
```

| 0.4.x | 1.x | Notes |
|---|---|---|
| `client.v1.object.sign(object_list=[...])` | `client.presign([...])` | Keyword becomes positional |
| returns an envelope with `.result` / `.status` / `.error` | returns the unwrapped data | See SDK-011 |
| non-zero `.status` returned, not raised | raises **`EverOSStorageError`** | An `if resp.status != 0:` branch becomes unreachable |

### Steps:
1. Rewrite the call and drop the `object_list=` keyword.
2. **Convert the status check into exception handling.** A caller that inspected
   `.status` silently stops handling storage failures otherwise.
3. Search patterns: `.v1.object.`, `object.sign(`, `object_list=`.

---

## SDK-018: Test doubles, fakes and fixtures

### Change Type: BREAKING - and the main source of false confidence

**No rule elsewhere covers this, and it is usually the largest single hand-edit in a
migration.** A fake that still returns the v1 shape keeps the suite green while production
is broken, which is exactly the outcome this whole rule set exists to prevent.

Every one of these has to move with the code:

| Double | What changes |
|---|---|
| A fake client exposing `v1.memories.*` | Flat facade verbs (SDK-005) |
| A fake returning an envelope | Returns the `*Data` payload directly (SDK-011) |
| `tasks.retrieve` fakes | `task_get` / `task_wait`, returning `TaskItem` with `.id` (SDK-016) |
| A fake for an async poller | Must fake `client.memory.add_memory` returning `SuccessEnvelopeAddData`, because SDK-016 routes that path through the generated client |
| Recorded responses (VCR cassettes, JSON fixtures, Postman) | Field renames from API-008: `raw_messages` -> `unprocessed_messages`, `agent_memory` -> two arrays, `request_id` moved to the envelope |
| `delete` fakes returning `None` | Return a `DeleteData` (`filters`, `count`) |
| `respx` / `httpx.MockTransport` / `httpx_mock` | **Stop intercepting entirely** — 1.x is on `urllib3` (SDK-003) |

### Steps:
1. Locate every double: `conftest.py`, `tests/**`, `**/fixtures/**`, `**/cassettes/**`,
   `*.postman_collection.json`, and any class whose name contains `Fake`, `Mock`, `Stub` or
   `Dummy` near an EverOS import.
2. Migrate each to the target shape. Where a double asserts on a field that moved, the
   assertion is the thing that has to change, not the production code.
3. If a double cannot be migrated because it covers a removed capability, mark the test
   `skip` with the migration reason. Do not delete it and do not leave it failing — the
   skip is the record of what the customer still has to decide.

---

## Applying the rules: order and hazards

There is no search-and-replace table in this file. The one that used to be here caused
more damage than it saved: `client.v1.memories.` -> `client.` also rewrites
`client.v1.memories.group.add(...)` and `client.v1.memories.agent.add(...)`, which
SDK-014 requires be left alone and flagged. Worse, it is self-concealing — once the
`.v1.` marker is gone, the blocker pass cannot find those call sites and the Impact
Report shows **zero** group calls on a codebase full of them.

Work rule by rule instead, in this order:

1. **PRE-001** — Python 3.12 floor. Stop here if it fails.
2. **Blocker inventory** — SDK-004, SDK-014, SDK-010's `memory_id`, SDK-003's removed
   kwargs. Record `file:line` for each **before** any rewrite, while the `.v1.` markers
   are still intact.
3. **SDK-013** — type imports, and **SDK-018** type-level doubles. In a typed codebase
   nothing else checks out until these are right.
4. **SDK-002 / SDK-003** — client construction.
5. **SDK-005 through SDK-011** — call sites and response access, one rule at a time.
   Restrict any `client.v1.memories.` rewrite to the five facade verbs explicitly:
   `add`, `search`, `get`, `flush`, `delete`.
6. **SDK-016 / SDK-017** — task polling and storage.
7. **SDK-012** — exception handling, after the call sites it has to wrap.
8. **SDK-018** — the remaining doubles and fixtures.
9. **SDK-001** — the dependency pin, **last**. Bumping it earlier makes an interrupted
   run look finished.

Portable across every rule: `episodic_memory` -> `episode`, and `raw_messages` ->
`unprocessed_messages` in response handling. Those two are safe as literal substitutions.
Nothing else in this file is.
