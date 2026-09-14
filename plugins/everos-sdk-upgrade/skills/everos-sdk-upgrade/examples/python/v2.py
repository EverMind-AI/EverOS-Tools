"""
EverOS SDK 1.x (everos-cloud, v2 Memory API) — canonical usage reference.

Naming note: the package version is 1.x; the API it calls is v2. This file is the
"v2" reference in the skill's version-chaining scheme. Verified against the published
1.1.0 wheel and against live prod behaviour on 2026-09-04.

Rule references point at migration/python/v1-to-v2.md (SDK-*) and
migration/http/v1-to-v2.md (API-*).
"""

import os
import time

from everos_cloud import EverOS, EverOSAPIError, EverOSError

USER_ID = "user-alice"
AGENT_ID = "agent-support-bot"
SESSION_ID = "session-1"


# SDK-002: api_key is REQUIRED (no env fallback in 1.x); base_url= is now host=.
# The EVER_OS_BASE_URL environment variable is NOT read automatically any more —
# omitting host= silently targets production.
def create_client() -> EverOS:
    return EverOS(
        api_key=os.environ["EVEROS_API_KEY"],
        host=os.environ.get("EVER_OS_BASE_URL"),  # None -> https://api.evermind.ai
        # API-005: scope defaults, inherited by every call on this client
        app_id="default",
        project_id="default",
        timeout=60.0,
    )
    # SDK-003: max_retries / http_client / default_headers are GONE. 0.4.x retried
    # twice by default; 1.x does not retry at all — wrap calls yourself if needed.


# SDK-006 / API-003 / API-004: add() rewrite.
#   - session_id is required and comes first
#   - the owner moved onto each message as sender_id
#   - timestamps are unix MILLISECONDS (a seconds value is a hard 422)
def add_memory(client: EverOS):
    now_ms = int(time.time() * 1000)
    result = client.add(
        session_id=SESSION_ID,
        messages=[
            {
                "sender_id": USER_ID,      # NOT a top-level user_id any more
                "sender_name": "Alice",    # optional display name; does not affect scoping
                "role": "user",
                "content": "I love hiking in the mountains",
                "timestamp": now_ms,       # milliseconds, not seconds
            },
            {
                "sender_id": AGENT_ID,     # an assistant turn is owned by the agent
                "role": "assistant",
                "content": "Noted — mountain hiking.",
                "timestamp": now_ms + 1000,
            },
        ],
        async_mode=False,  # sync: deterministic, and extraction runs immediately
    )
    # SDK-011: the facade returns .data already unwrapped
    print(f"message_count={result.message_count}, status={result.status}")
    return result


# SDK-007 / API-011: flush is keyed by session_id (0.4.x used user_id).
# After a synchronous add, extraction already ran, so this returns "no_extraction" —
# that is success, not failure.
def flush_session(client: EverOS):
    result = client.flush(SESSION_ID)
    if result.status not in ("extracted", "no_extraction"):
        raise RuntimeError(f"unexpected flush status: {result.status}")
    return result


# SDK-009 / API-007: get() — memory_type first, values renamed.
# A user owner may only ask for "episode" or "profile".
def get_episodes(client: EverOS):
    result = client.get("episode", user_id=USER_ID, page=1, page_size=20)
    print(f"total={result.total_count}")
    for ep in result.episodes:
        print(f"  - {ep.summary}")
    return result


def get_profile(client: EverOS):
    result = client.get("profile", user_id=USER_ID)
    for p in result.profiles:
        print(f"  - {p.profile_data}")
    return result


# API-015: agent memory is read with agent_id + agent_case / agent_skill.
# An agent owner may ONLY ask for those two types (a mismatch is a 422).
def get_agent_cases(client: EverOS):
    return client.get("agent_case", agent_id=AGENT_ID)


# SDK-008: search() — query first, scope as a keyword arg.
# Exactly one of user_id / agent_id is required.
def search_memories(client: EverOS):
    result = client.search(
        "outdoor hobbies",
        user_id=USER_ID,
        method="hybrid",  # keyword | vector | hybrid (default) | agentic
        top_k=5,
    )
    for ep in result.episodes:
        print(f"  - score={ep.score} {ep.summary}")
    # API-008: raw_messages -> unprocessed_messages;
    #          agent_memory -> agent_cases + agent_skills
    for m in result.unprocessed_messages:
        print(f"  - unprocessed: {m}")
    return result


# SDK-010 / API-009: delete() is keyword-only and returns a body (0.4.x returned 204/None).
# Scope matters: user_id alone removes the profile too; adding session_id does not.
def delete_user(client: EverOS):
    result = client.delete(user_id=USER_ID)
    print(f"deleted {result.count} via {result.filters}")
    return result


def delete_session_only(client: EverOS):
    # Removes what this session produced. The user's profile SURVIVES this call.
    return client.delete(user_id=USER_ID, session_id=SESSION_ID)


# New in v2: bulk profile editing (no 0.4.x equivalent).
def edit_profile(client: EverOS):
    return client.edit(
        user_id=USER_ID,
        operations=[
            {"action": "add", "category": "Preferences", "description": "Vegetarian"},
        ],
    )


# SDK-012: the granular exception classes are gone; branch on .status instead.
def handle_errors(client: EverOS):
    try:
        client.search("test", user_id=USER_ID)
    except EverOSAPIError as e:
        if e.status == 403:
            print("account is not enabled for the v2 API (VERSION_NOT_ALLOWED)")
        elif e.status == 422:
            print(f"bad request: {e.body}")
        elif e.status == 429:
            print("quota exceeded")
        else:
            raise
    except EverOSError:
        # still the base class — `except EverOSError` from 0.4.x keeps working
        raise


# SDK-016 / API-018: async writes and task polling.
# The facade's add() returns .data, which carries NO task id -- AddData has only
# message_count and status. The id to poll with is the envelope's request_id, so an
# async caller has to go through the generated client to see it.
def add_async_and_wait(client: EverOS):
    from everos_cloud.models.add_input import AddInput
    from everos_cloud.models.message_item import MessageItem
    from everos_cloud.models.content import Content

    envelope = client.memory.add_memory(AddInput(
        app_id="default", project_id="default",
        session_id=SESSION_ID, async_mode=True,
        messages=[
            MessageItem(
                sender_id=USER_ID, role="user", timestamp=int(time.time() * 1000),
                # The generated client does NOT coerce str -> Content the way the
                # facade does, so build it explicitly or pydantic rejects the call.
                content=Content("I love hiking in the mountains"),
            )
        ],
    ))

    task_id = envelope.request_id          # NOT envelope.data.task_id
    task = client.task_wait(task_id, timeout=180, interval=3)

    # v2 statuses: queued | pending | processing | success | failed.
    # Only success and failed are terminal. "completed" is a v1 value and is never
    # returned, so a stale check against it silently polls until it times out.
    if task.status == "success":
        print(f"task {task.id} finished ({task.task_type})")
    return task


# Hand-rolled equivalent, if you need the loop yourself
def poll_by_hand(client: EverOS, task_id: str):
    while True:
        task = client.task_get(task_id)    # returns an unwrapped TaskItem
        if task.status in ("success", "failed"):
            return task
        time.sleep(3)


# SDK-015: the generated low-level clients, when the facade omits something.
# They return the full envelope (so request_id is reachable) and raise ApiException.
def low_level_access(client: EverOS):
    envelope = client.memory.get_memory(
        {"memory_type": "episode", "user_id": USER_ID, "app_id": "default", "project_id": "default"}
    )
    print(f"request_id={envelope.request_id}")
    return envelope.data


def main() -> None:
    with create_client() as client:  # close() releases pooled connections
        add_memory(client)
        flush_session(client)
        get_episodes(client)
        get_profile(client)
        search_memories(client)
        delete_user(client)


if __name__ == "__main__":
    main()
