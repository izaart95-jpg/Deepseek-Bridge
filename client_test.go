package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
)

// TestLiveChat mirrors tests/test_api.py: requires DEEPSEEK_TOKEN.
// Skips when the token is not set.
func TestLiveChat(t *testing.T) {
	token := os.Getenv("DEEPSEEK_TOKEN")
	if token == "" {
		t.Skip("DEEPSEEK_TOKEN not set")
	}

	api, err := NewDeepSeekAPI(token)
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	ctx := context.Background()

	sessionID, err := api.CreateChatSession(ctx)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	t.Logf("session: %s", sessionID)

	// first message in thread
	var parentID string
	var first strings.Builder
	err = api.ChatCompletion(ctx, ChatParams{
		ChatSessionID:   sessionID,
		Prompt:          "Tell me about neural networks",
		ThinkingEnabled: true,
		SearchEnabled:   false,
	}, func(ch Chunk) bool {
		if ch.Type == "ready" && ch.ResponseMessageID != nil {
			parentID = fmt.Sprint(ch.ResponseMessageID)
		}
		if ch.Type == "content" {
			first.WriteString(ch.Content)
		}
		if ch.Type == "complete" {
			t.Log("first response complete")
		}
		return false
	})
	if err != nil {
		t.Fatalf("first completion: %v", err)
	}
	if first.Len() == 0 {
		t.Fatal("first response was empty")
	}
	t.Logf("first response (%d chars): %s", first.Len(), truncate(first.String(), 120))
	if parentID == "" {
		parentID = sessionID
	}

	// follow-up in same thread
	var second strings.Builder
	err = api.ChatCompletion(ctx, ChatParams{
		ChatSessionID:   sessionID,
		Prompt:          "How do they compare to other ML models?",
		ParentMessageID: parentID,
		ThinkingEnabled: true,
		SearchEnabled:   false,
	}, func(ch Chunk) bool {
		if ch.Type == "content" {
			second.WriteString(ch.Content)
		}
		return false
	})
	if err != nil {
		t.Fatalf("follow-up completion: %v", err)
	}
	if second.Len() == 0 {
		t.Fatal("follow-up response was empty")
	}
	t.Logf("follow-up response (%d chars): %s", second.Len(), truncate(second.String(), 120))
}

// TestPowSolver verifies the WASM PoW solver end-to-end (needs network for
// the challenge but not a valid token for this endpoint).
func TestPowSolver(t *testing.T) {
	token := os.Getenv("DEEPSEEK_TOKEN")
	if token == "" {
		t.Skip("DEEPSEEK_TOKEN not set")
	}
	api, err := NewDeepSeekAPI(token)
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	challenge, err := api.GetPowChallenge(context.Background())
	if err != nil {
		t.Fatalf("get challenge: %v", err)
	}
	resp, err := api.pow.SolveChallenge(*challenge)
	if err != nil {
		t.Fatalf("solve: %v", err)
	}
	if resp == "" {
		t.Fatal("empty pow response")
	}
	t.Logf("pow response (len %d)", len(resp))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
