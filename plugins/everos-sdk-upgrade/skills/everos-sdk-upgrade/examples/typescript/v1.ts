/**
 * EverOS Cloud API v1 — canonical raw-HTTP reference (TypeScript).
 *
 * This is the "before" shape for the v1 -> v2 hop. Diff a migrated file against
 * v2.ts, not against this one.
 *
 * Note how little of this contains a literal path: `memoryBase` is assembled from
 * a version constant, which is why a detection pattern anchored on
 * "/api/v1/memories" finds nothing in a file like this.
 */

const BASE = process.env.EVEROS_BASE_URL ?? "https://api.evermind.ai";
const API_VERSION = "v1";
const memoryBase = `/api/${API_VERSION}/memories`;

const headers = {
  Authorization: `Bearer ${process.env.EVEROS_API_KEY}`,
  "Content-Type": "application/json",
};

interface Envelope<T> {
  data: T;
}

interface AddResult {
  task_id: string;
  message_count: number;
  status: string;
  message: string;
}

interface Episode {
  id: string;
  user_id: string;
  summary: string;
  score?: number;
}

interface SearchResult {
  episodes: Episode[];
  profiles: unknown[];
  raw_messages: unknown[];
  agent_memory: unknown | null;
}

async function post<T>(path: string, body: unknown): Promise<Envelope<T>> {
  const res = await fetch(`${BASE}${path}`, {
    method: "POST",
    headers,
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    const err = (await res.json()) as { code: string; message: string };
    throw new Error(`${err.code}: ${err.message}`);
  }
  return (await res.json()) as Envelope<T>;
}

/** One top-level user_id; messages carry no sender. */
export async function addMemory(userId: string, sessionId: string, text: string) {
  const body = {
    user_id: userId,
    session_id: sessionId,
    async_mode: true,
    messages: [
      {
        role: "user",
        content: text,
        timestamp: Math.floor(Date.now() / 1000), // seconds
      },
    ],
  };
  const res = await post<AddResult>(memoryBase, body);
  return res.data.task_id;
}

export async function pollTask(taskId: string) {
  const res = await fetch(`${BASE}/api/${API_VERSION}/tasks/${taskId}`, { headers });
  const body = (await res.json()) as Envelope<{ status: string }>;
  return body.data.status;
}

export async function searchMemories(userId: string, query: string) {
  const res = await post<SearchResult>(`${memoryBase}/search`, {
    filters: { user_id: userId },
    query,
    memory_types: ["episodic_memory", "profile"],
    top_k: 5,
  });
  return res.data.episodes;
}

export async function getEpisodes(userId: string) {
  const res = await post<{ episodes: Episode[]; total_count: number }>(`${memoryBase}/get`, {
    memory_type: "episodic_memory",
    filters: { user_id: userId },
    page: 1,
    page_size: 20,
  });
  return res.data.episodes;
}

export async function deleteMemory(memoryId: string) {
  await post<null>(`${memoryBase}/delete`, { memory_id: memoryId });
}

/** Group memory: multi-party, addressed by group_id. */
export async function addGroupMemory(groupId: string, msgs: unknown[]) {
  return post<AddResult>(`${memoryBase}/group`, { group_id: groupId, messages: msgs });
}
