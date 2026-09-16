//go:build ignore

// Package everos is the canonical EverOS Cloud API v1 raw-HTTP reference (Go).
//
// This is the "before" shape for the v1 -> v2 hop. Diff a migrated file against
// v2.go, not against this one.
//
// As in the TypeScript reference, note that no full path literal appears here:
// the roots are assembled from apiVersion, which is why a detection pattern
// anchored on "/api/v1/memories" finds nothing in an idiomatic Go caller.
package everos

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

const (
	apiVersion = "v1"
	memoryBase = "/api/" + apiVersion + "/memories"
	groupBase  = "/api/" + apiVersion + "/memories/group"
	taskBase   = "/api/" + apiVersion + "/tasks/"
)

type Envelope[T any] struct {
	Data T `json:"data"`
}

type AddResult struct {
	TaskID       string `json:"task_id"`
	MessageCount int    `json:"message_count"`
	Status       string `json:"status"`
	Message      string `json:"message"`
}

type Episode struct {
	ID      string   `json:"id"`
	UserID  string   `json:"user_id"`
	Summary string   `json:"summary"`
	Score   *float64 `json:"score,omitempty"`
}

type SearchResult struct {
	Episodes    []Episode `json:"episodes"`
	RawMessages []any     `json:"raw_messages"`
	AgentMemory any       `json:"agent_memory"`
}

type Message struct {
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
		var e struct{ Code, Message string }
		json.NewDecoder(resp.Body).Decode(&e)
		return nil, fmt.Errorf("%s: %s", e.Code, e.Message)
	}
	var out Envelope[T]
	return &out, json.NewDecoder(resp.Body).Decode(&out)
}

// AddMemory: one top-level user_id, messages carry no sender.
func (c *Client) AddMemory(userID, sessionID, text string) (string, error) {
	body := map[string]any{
		"user_id":    userID,
		"session_id": sessionID,
		"async_mode": true,
		"messages": []Message{
			{Role: "user", Content: text, Timestamp: time.Now().Unix()}, // seconds
		},
	}
	res, err := post[AddResult](c, memoryBase, body)
	if err != nil {
		return "", err
	}
	return res.Data.TaskID, nil
}

func (c *Client) SearchMemories(userID, query string) ([]Episode, error) {
	res, err := post[SearchResult](c, memoryBase+"/search", map[string]any{
		"filters":      map[string]string{"user_id": userID},
		"query":        query,
		"memory_types": []string{"episodic_memory", "profile"},
		"top_k":        5,
	})
	if err != nil {
		return nil, err
	}
	return res.Data.Episodes, nil
}

func (c *Client) DeleteMemory(memoryID string) error {
	_, err := post[any](c, memoryBase+"/delete", map[string]any{"memory_id": memoryID})
	return err
}

func (c *Client) AddGroupMemory(groupID string, msgs []Message) (*Envelope[AddResult], error) {
	return post[AddResult](c, groupBase, map[string]any{"group_id": groupID, "messages": msgs})
}
