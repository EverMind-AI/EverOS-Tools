/**
 * EverOS Cloud API v2 — canonical raw-HTTP reference (TypeScript).
 *
 * Rule references are to migration/http/v1-to-v2.md (API-0NN).
 * This is the diff target for a migrated TypeScript caller.
 */

const BASE = process.env.EVEROS_BASE_URL ?? "https://api.evermind.ai";

// API-001 step 4: do NOT bump a shared version constant. Split it, so the endpoints
// that were removed in v2 stay pinned to v1 and fail as a visible blocker rather
// than as a 404 against a path that never existed.
const API_VERSION = "v2";
const LEGACY_API_VERSION = "v1"; // EVEROS-MIGRATION: removed in v2, see API-012
const memoryBase = `/api/${API_VERSION}/memory`; // note: singular "memory" in v2

// API-003: a raw caller must supply sender_id on every message. v1 had no agent id
// anywhere, so one has to be introduced. Make it configurable and confirm the value
// with the customer: per API-015 it decides whether a write becomes agent memory.
const AGENT_ID = process.env.EVEROS_AGENT_ID ?? "acme-assistant";

const headers = {
  Authorization: `Bearer ${process.env.EVEROS_API_KEY}`,
  "Content-Type": "application/json",
};

// API-008: `data` stays on the wire. Only request_id moved to the envelope.
// (The Python SDK returns .data pre-unwrapped — that is an SDK convenience, SDK-011,
// and does not apply here.)
interface Envelope<T> {
  request_id: string;
  data: T;
}

interface AddData {
  message_count: number;
  status: string; // accumulated | extracted | queued
}

interface Episode {
  id: string;
  user_id: string;
  session_id: string;
  sender_ids: string[];
  summary: string;
  score?: number;
}

interface SearchData {
  episodes: Episode[];
  profiles: unknown[];
  agent_cases: unknown[];     // API-008: agent_memory split into two arrays
  agent_skills: unknown[];
  unprocessed_messages: unknown[]; // API-008: was raw_messages
}

interface TaskItem {
  id: string;
  status: "queued" | "pending" | "processing" | "success" | "failed";
  task_type: string;
  created_at: string;
}

async function post<T>(path: string, body: unknown): Promise<Envelope<T>> {
  const res = await fetch(`${BASE}${path}`, {
    method: "POST",
    headers,
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    // API-010: three body shapes are in use. Branch on the HTTP status and unwrap
    // defensively; `body.code` is absent on 401 and on validation errors.
    const raw = (await res.json()) as Record<string, any>;
    const err = raw.error ?? raw;
    throw new Error(`HTTP ${res.status} ${err.code ?? "unknown"}: ${err.message ?? ""}`);
  }
  return (await res.json()) as Envelope<T>;
}

export async function addMemory(userId: string, sessionId: string, text: string) {
  const body = {
    app_id: "default",
    project_id: "default",
    session_id: sessionId, // API-003: required, 1-128 chars
    async_mode: true,      // unchanged from the caller's v1 value
    messages: [
      {
        sender_id: userId, // API-003: the owner moved onto each message
        role: "user",
        content: text,
        timestamp: Date.now(), // API-004: MILLISECONDS. Date.now() already is.
      },
    ],
  };
  const res = await post<AddData>(`${memoryBase}/add`, body);
  // API-018: the add response carries no task id. Poll with the envelope's request_id.
  return res.request_id;
}

/** An assistant turn is owned by the agent, not by the human. See API-003 / API-015. */
export async function addAssistantTurn(sessionId: string, text: string) {
  return post<AddData>(`${memoryBase}/add`, {
    session_id: sessionId,
    messages: [
      { sender_id: AGENT_ID, role: "assistant", content: text, timestamp: Date.now() },
    ],
  });
}

export async function pollTask(taskId: string): Promise<TaskItem> {
  const res = await fetch(`${BASE}/api/${API_VERSION}/tasks/${taskId}`, { headers });
  const body = (await res.json()) as Envelope<TaskItem>;
  // API-018: only success and failed are terminal. queued / pending / processing
  // all mean "keep waiting" — treating processing as terminal reports a task done
  // before it is.
  return body.data;
}

export async function searchMemories(userId: string, query: string) {
  // API-006: filters{} is gone; exactly one of user_id / agent_id is required.
  // API-007/SDK-008: memory_types was removed. The response still separates the
  // kinds, so filter by type on the way out.
  const res = await post<SearchData>(`${memoryBase}/search`, {
    user_id: userId,
    query,
    method: "hybrid",
    top_k: 5,
    include_profile: true,
  });
  return res.data.episodes;
}

export async function getEpisodes(userId: string) {
  const res = await post<{ episodes: Episode[]; total_count: number }>(`${memoryBase}/get`, {
    memory_type: "episode", // API-007: was episodic_memory
    user_id: userId,        // API-006: promoted out of filters{}
    page: 1,
    page_size: 20,
  });
  return res.data.episodes;
}

export async function deleteUser(userId: string) {
  // API-009: scope-based only, and the response now has a body.
  // Deleting by user_id alone also removes the profile; adding session_id does not.
  const res = await post<{ filters: string[]; count: number }>(`${memoryBase}/delete`, {
    user_id: userId,
  });
  return res.data.count;
}

// EVEROS-MIGRATION (API-009): v2 has no single-memory delete. DeleteInput accepts only
// user_id / agent_id / session_id. The nearest option is a session-scoped delete, which
// is coarser. This cannot be migrated automatically.
//
// EVEROS-MIGRATION (API-012): group memory has no v2 equivalent. Multi-party
// conversations still work — write all participants into one session_id and every
// episode carries all of them in sender_ids — but a group is no longer an addressable
// object, so reads fan out per participant. Contact EverOS before changing this.
//
// The v1 types these still need are re-declared locally rather than left dangling,
// because a dangling type reference is a compile error that takes down consumers which
// never touched EverOS.
interface LegacyEnvelope<T> { data: T; }
interface LegacyAddResult { task_id: string; message_count: number; status: string; }

export async function addGroupMemory(groupId: string, msgs: unknown[]) {
  const res = await fetch(`${BASE}/api/${LEGACY_API_VERSION}/memories/group`, {
    method: "POST",
    headers,
    body: JSON.stringify({ group_id: groupId, messages: msgs }),
  });
  return (await res.json()) as LegacyEnvelope<LegacyAddResult>;
}
