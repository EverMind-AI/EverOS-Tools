//go:build ignore

// Package everos is the canonical EverOS Cloud API v2 raw-HTTP reference (Go).
//
// Rule references are to migration/http/v1-to-v2.md (API-0NN).
// This is the diff target for a migrated Go caller.
package everos

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

// API-001 step 4: do NOT bump a shared version constant. Splitting it keeps the
// endpoints removed in v2 pinned to v1, so they fail as a visible blocker instead
// of 404-ing against a v2 path that never existed.
const (
	apiVersion       = "v2"
	legacyAPIVersion = "v1" // EVEROS-MIGRATION: removed in v2, see API-012
	memoryBase       = "/api/" + apiVersion + "/memory" // singular in v2
	taskBase         = "/api/" + apiVersion + "/tasks/"
	legacyGroupBase  = "/api/" + legacyAPIVersion + "/memories/group"
)

// API-003: a raw caller supplies sender_id on every message. v1 had no agent id
// anywhere, so one has to be introduced; per API-015 it decides whether a write
// becomes agent memory, so confirm the value with the customer.
func agentID() string {
	if v := os.Getenv("EVEROS_AGENT_ID"); v != "" {
		return v
	}
	return "acme-assistant"
}

// API-008: `data` stays on the wire; only request_id moved to the envelope.
type Envelope[T any] struct {
	RequestID string `json:"request_id"`
	Data      T      `json:"data"`
}

type AddData struct {
	MessageCount int    `json:"message_count"`
	Status       string `json:"status"` // accumulated | extracted | queued
}

type Episode struct {
	ID        string   `json:"id"`
	UserID    string   `json:"user_id"`
	SessionID string   `json:"session_id"`
	SenderIDs []string `json:"sender_ids"`
	Summary   string   `json:"summary"`
	Score     *float64 `json:"score,omitempty"`
}

type SearchData struct {
	Episodes            []Episode `json:"episodes"`
	Profiles            []any     `json:"profiles"`
	AgentCases          []any     `json:"agent_cases"`           // API-008: agent_memory split
	AgentSkills         []any     `json:"agent_skills"`          // into two arrays
	UnprocessedMessages []any     `json:"unprocessed_messages"`  // API-008: was raw_messages
}

type TaskItem struct {
	ID        string `json:"id"`
	Status    string `json:"status"` // queued|pending|processing|success|failed
	TaskType  string `json:"task_type"`
	CreatedAt string `json:"created_at"`
}

type Message struct {
	SenderID  string `json:"sender_id"` // API-003: the owner moved onto each message
	Role      string `json:"role"`
	Content   string `json:"content"`
	Timestamp int64  `json:"timestamp"`
}

type Client struct {
	BaseURL string
	APIKey  string
	HTTP    *http.Client
}

func New() *Client {
	base := os.Getenv("EVEROS_BASE_URL")
	if base == "" {
		base = "https://api.evermind.ai"
	}
	return &Client{BaseURL: base, APIKey: os.Getenv("EVEROS_API_KEY"), HTTP: http.DefaultClient}
}

// API-010: three error body shapes are in use. Branch on the HTTP status and unwrap
// defensively — `code` is at the top level for 400/404/422-InvalidParameter, and
// nested under `error` for 401 and for request-model validation failures.
type errBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Error   *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func (e errBody) unwrap() (string, string) {
	if e.Error != nil {
		return e.Error.Code, e.Error.Message
	}
	return e.Code, e.Message
}

func post[T any](c *Client, path string, body any) (*Envelope[T], error) {
	buf, _ := json.Marshal(body)
	req, err := http.NewRequest("POST", c.BaseURL+path, bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		var e errBody
		json.NewDecoder(resp.Body).Decode(&e)
		code, msg := e.unwrap()
		return nil, fmt.Errorf("HTTP %d %s: %s", resp.StatusCode, code, msg)
	}
	var out Envelope[T]
	return &out, json.NewDecoder(resp.Body).Decode(&out)
}

func (c *Client) AddMemory(userID, sessionID, text string) (string, error) {
	body := map[string]any{
		"app_id":     "default",
		"project_id": "default",
		"session_id": sessionID, // API-003: required, 1-128 chars
		"async_mode": true,      // unchanged from the caller's v1 value
		"messages": []Message{{
			SenderID:  userID,
			Role:      "user",
			Content:   text,
			Timestamp: time.Now().UnixMilli(), // API-004: MILLISECONDS, was .Unix()
		}},
	}
	res, err := post[AddData](c, memoryBase+"/add", body)
	if err != nil {
		return "", err
	}
	// API-018: the add response carries no task id. Poll with the envelope's request_id.
	return res.RequestID, nil
}

// AddAssistantTurn: an assistant turn is owned by the agent. See API-003 / API-015.
func (c *Client) AddAssistantTurn(sessionID, text string) (*Envelope[AddData], error) {
	return post[AddData](c, memoryBase+"/add", map[string]any{
		"session_id": sessionID,
		"messages": []Message{{
			SenderID: agentID(), Role: "assistant", Content: text,
			Timestamp: time.Now().UnixMilli(),
		}},
	})
}

// PollTask: API-018. Only success and failed are terminal; queued, pending and
// processing all mean keep waiting.
func (c *Client) PollTask(taskID string) (*TaskItem, error) {
	req, err := http.NewRequest("GET", c.BaseURL+taskBase+taskID, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out Envelope[TaskItem]
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out.Data, nil
}

func TaskIsTerminal(status string) bool {
	return status == "success" || status == "failed"
}

// API-006: filters{} is gone; exactly one of user_id / agent_id is required.
// memory_types was removed, so filter by kind on the way out.
func (c *Client) SearchMemories(userID, query string) ([]Episode, error) {
	res, err := post[SearchData](c, memoryBase+"/search", map[string]any{
		"user_id":         userID,
		"query":           query,
		"method":          "hybrid",
		"top_k":           5,
		"include_profile": true,
	})
	if err != nil {
		return nil, err
	}
	return res.Data.Episodes, nil
}

func (c *Client) GetEpisodes(userID string) ([]Episode, error) {
	res, err := post[SearchData](c, memoryBase+"/get", map[string]any{
		"memory_type": "episode", // API-007: was episodic_memory
		"user_id":     userID,    // API-006: promoted out of filters{}
		"page":        1,
		"page_size":   20,
	})
	if err != nil {
		return nil, err
	}
	return res.Data.Episodes, nil
}

// API-009: scope-based delete only, and the response now carries a body.
// user_id alone also removes the profile; adding session_id does not.
func (c *Client) DeleteUser(userID string) (int, error) {
	res, err := post[struct {
		Filters []string `json:"filters"`
		Count   int      `json:"count"`
	}](c, memoryBase+"/delete", map[string]any{"user_id": userID})
	if err != nil {
		return 0, err
	}
	return res.Data.Count, nil
}

// EVEROS-MIGRATION (API-009): v2 has no single-memory delete. DeleteInput accepts only
// user_id / agent_id / session_id. The nearest option is a session-scoped delete, which
// is coarser. Cannot be migrated automatically.
//
// EVEROS-MIGRATION (API-012): group memory has no v2 equivalent. Multi-party
// conversations still work — write every participant into one session_id and each
// episode carries them all in SenderIDs — but a group is no longer addressable, so
// reads fan out per participant. Contact EverOS before changing this.
//
// The v1 types below are re-declared locally rather than left pointing at the renamed
// v2 types: a dangling type reference is a compile error that takes down packages which
// never touched EverOS.
type LegacyEnvelope[T any] struct {
	Data T `json:"data"`
}

type LegacyAddResult struct {
	TaskID       string `json:"task_id"`
	MessageCount int    `json:"message_count"`
	Status       string `json:"status"`
}

func (c *Client) AddGroupMemory(groupID string, msgs []Message) (*LegacyEnvelope[LegacyAddResult], error) {
	buf, _ := json.Marshal(map[string]any{"group_id": groupID, "messages": msgs})
	req, err := http.NewRequest("POST", c.BaseURL+legacyGroupBase, bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out LegacyEnvelope[LegacyAddResult]
	return &out, json.NewDecoder(resp.Body).Decode(&out)
}
