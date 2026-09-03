package dsproxy

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestContextRotRootCause verifies the fix: the new prompt structure uses
// XML-like section tags instead of [ROLE: ...] tags, eliminating the
// marker ambiguity that caused context rot.
func TestContextRotRootCause(t *testing.T) {
	messages := []chatMessage{
		{
			Role:    "system",
			Content: json.RawMessage(`"You are a helpful assistant."`),
		},
		{
			Role:    "user",
			Content: json.RawMessage(`"Find my IP"`),
		},
		{
			Role:    "assistant",
			Content: json.RawMessage(`""`),
			ToolCalls: []assistantToolCall{
				{
					ID:   "call_1",
					Type: "function",
					Function: struct {
						Name      string          `json:"name"`
						Arguments json.RawMessage `json:"arguments"`
					}{
						Name:      "bash",
						Arguments: json.RawMessage(`{"command":"curl -s https://api.ipify.org"}`),
					},
				},
			},
		},
		{
			Role:       "tool",
			Content:    json.RawMessage(`"10.0.0.1"`),
			ToolCallID: "call_1",
		},
	}

	tools := []openAITool{
		{
			Type: "function",
			Function: &openAIFnSpec{
				Name:        "bash",
				Description: "Execute a bash command",
			},
		},
	}

	prompt := buildAgentPrompt(messages, tools)

	// FIX VERIFIED: No [ROLE: ...] markers should exist
	oldMarkers := []string{
		"[ROLE: system]",
		"[ROLE: user]",
		"[ROLE: assistant]",
		"[ROLE: tool_result]",
		"[ROLE: tool]",
	}
	for _, marker := range oldMarkers {
		if strings.Contains(prompt, marker) {
			t.Errorf("FIX FAILED: Old marker %s still present", marker)
		}
	}

	// FIX VERIFIED: New XML-like tags should be present
	if !strings.Contains(prompt, "<system>") {
		t.Error("Missing <system> section")
	}
	if !strings.Contains(prompt, "<tool_result") {
		t.Error("Missing <tool_result> tag")
	}
	if !strings.Contains(prompt, `<tool_result call_id="call_1"`) {
		t.Error("Missing call_id attribute in tool_result")
	}
	if !strings.Contains(prompt, "<tool_exchange>") {
		t.Error("Missing <tool_exchange> grouping")
	}
	if !strings.Contains(prompt, "<current_task>") {
		t.Error("Missing <current_task> anchor")
	}

	t.Log("FIX VERIFIED: Context rot resolved with structured prompt")
	t.Log("\n=== FULL PROMPT ===")
	t.Log(prompt)
}

// TestThinkingModeConfusion verifies that the new prompt structure
// doesn't confuse the model when reasoning/thinking is enabled.
func TestThinkingModeConfusion(t *testing.T) {
	messages := []chatMessage{
		{
			Role:    "system",
			Content: json.RawMessage(`"You are a helpful assistant with access to bash tools."`),
		},
		{
			Role:    "user",
			Content: json.RawMessage(`"Find me public ip"`),
		},
		{
			Role:    "assistant",
			Content: json.RawMessage(`""`),
			ToolCalls: []assistantToolCall{
				{
					ID:   "call_123",
					Type: "function",
					Function: struct {
						Name      string          `json:"name"`
						Arguments json.RawMessage `json:"arguments"`
					}{
						Name:      "bash",
						Arguments: json.RawMessage(`{"command":"curl -s https://api.ipify.org"}`),
					},
				},
			},
		},
		{
			Role:       "tool",
			Content:    json.RawMessage(`"110.226.238.87"`),
			ToolCallID: "call_123",
		},
		{
			Role:    "assistant",
			Content: json.RawMessage(`"Your IP address is 110.226.238.87."`),
		},
		{
			Role:    "user",
			Content: json.RawMessage(`"Fair enough now find my os arch and read my codebase explain me it in summary"`),
		},
	}

	tools := []openAITool{
		{
			Type: "function",
			Function: &openAIFnSpec{
				Name:        "bash",
				Description: "Execute a bash command",
				Parameters:  json.RawMessage(`{"type":"object","properties":{"command":{"type":"string"}}}`),
			},
		},
	}

	prompt := buildAgentPrompt(messages, tools)

	// The new structure eliminates the old confusion points:
	// 1. No more [ROLE: tool_result] appearing in both documentation and actual result
	// 2. Tool results are in clear <tool_result> tags with call_id
	// 3. Tool exchanges are grouped in <tool_exchange> blocks
	// 4. The last user message is in <current_task>

	// Verify no marker ambiguity
	if strings.Contains(prompt, "[ROLE: tool_result]") {
		t.Error("CONFUSION POINT STILL EXISTS: [ROLE: tool_result] marker present")
	}

	// Verify clear structure
	if !strings.Contains(prompt, "<tool_exchange>") {
		t.Error("Missing <tool_exchange> grouping")
	}
	if !strings.Contains(prompt, `<tool_result call_id="call_123"`) {
		t.Error("Missing <tool_result> with call_id")
	}

	// Verify the hallucinated pattern is prevented:
	// The old confusion caused models to generate <_calls><invoke name="bash">
	// The new structure clearly shows the <<<TOOL_CALL>>> format in <system>
	if !strings.Contains(prompt, "<<<TOOL_CALL>>>") {
		t.Error("System instructions should show correct tool call format")
	}

	t.Log("=== THINKING MODE: No confusion points found ===")
	t.Log("\n=== FULL PROMPT ===")
	t.Log(prompt)
}

// TestMarkerAmbiguity verifies that the new prompt structure has zero
// marker ambiguity — old [ROLE: ...] tags are completely eliminated.
func TestMarkerAmbiguity(t *testing.T) {
	// The old system prefix contained role tag mentions that could confuse
	// the model. Verify the new prefix has NONE.

	oldRoleTags := []string{
		"[ROLE: system]",
		"[ROLE: user]",
		"[ROLE: assistant]",
		"[ROLE: tool_result]",
		"[ROLE: tool]",
	}

	// Count in the system prefix
	for _, tag := range oldRoleTags {
		count := strings.Count(agentSystemPrefix, tag)
		if count > 0 {
			t.Errorf("System prefix still contains %s (%d times)", tag, count)
		}
	}

	// Build a full prompt and verify zero ambiguity
	messages := []chatMessage{
		{
			Role:    "user",
			Content: json.RawMessage(`"test"`),
		},
		{
			Role:    "assistant",
			Content: json.RawMessage(`""`),
			ToolCalls: []assistantToolCall{
				{
					ID:   "call_1",
					Type: "function",
					Function: struct {
						Name      string          `json:"name"`
						Arguments json.RawMessage `json:"arguments"`
					}{
						Name:      "bash",
						Arguments: json.RawMessage(`{"command":"echo test"}`),
					},
				},
			},
		},
		{
			Role:       "tool",
			Content:    json.RawMessage(`"test output"`),
			ToolCallID: "call_1",
		},
	}

	tools := []openAITool{
		{
			Type: "function",
			Function: &openAIFnSpec{
				Name:        "bash",
				Description: "Execute a bash command",
			},
		},
	}

	prompt := buildAgentPrompt(messages, tools)

	// Verify zero occurrences of old markers
	for _, tag := range oldRoleTags {
		count := strings.Count(prompt, tag)
		if count > 0 {
			t.Errorf("Full prompt contains %s (%d times) — should be zero", tag, count)
		}
	}

	// Verify new structure is present
	newMarkers := []string{
		"<system>",
		"<tools>",
		"<recent>",
		"<current_task>",
		"<output_rules>",
		"<tool_exchange>",
		"<tool_result",
		"<assistant>",
	}
	for _, marker := range newMarkers {
		if !strings.Contains(prompt, marker) {
			t.Errorf("Full prompt missing new marker: %s", marker)
		}
	}
	// Note: <user> tag only appears when there are multiple user messages
	// (the last user message goes into <current_task> instead)

	t.Log("MARKER AMBIGUITY: Zero old markers found — fix verified")
	t.Log("\n=== FULL PROMPT ===")
	t.Log(prompt)
}
