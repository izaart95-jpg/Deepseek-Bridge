package dsproxy

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestContextRotRootCause identifies the root cause of context rot:
// The [ROLE: tool_result] marker appears in BOTH the system instructions
// AND the actual tool result, causing model confusion.
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

	// ROOT CAUSE: Count occurrences of [ROLE: tool_result]
	toolResultCount := strings.Count(prompt, "[ROLE: tool_result]")
	t.Logf("[ROLE: tool_result] appears %d times in prompt", toolResultCount)

	// Find all occurrences and their context
	lines := strings.Split(prompt, "\n")
	for i, line := range lines {
		if strings.Contains(line, "[ROLE: tool_result]") {
			t.Logf("Line %d: %s", i, line)
		}
	}

	// The system prefix mentions [ROLE: tool_result] as documentation
	// This creates ambiguity for the model
	if toolResultCount > 1 {
		t.Log("ROOT CAUSE IDENTIFIED: [ROLE: tool_result] appears multiple times")
		t.Log("The system instructions mention it as documentation,")
		t.Log("AND the actual tool result uses the same marker.")
		t.Log("This causes model confusion about which is the real tool result.")
	}
}

// TestThinkingModeConfusion tests if thinking mode exacerbates the issue.
// When reasoning is enabled, the model's "thinking" might get confused
// about the conversation structure.
func TestThinkingModeConfusion(t *testing.T) {
	// This test documents the issue but cannot actually call the API.
	// It shows the prompt structure that causes confusion.

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

	// Analyze potential confusion points
	t.Log("=== THINKING MODE CONFUSION ANALYSIS ===")

	// 1. The system prefix explains the role tags
	// 2. The actual messages use the same role tags
	// 3. When thinking is enabled, the model might:
	//    - Try to "think" about the conversation structure
	//    - Get confused by the repeated [ROLE: ...] markers
	//    - Generate hallucinated tool calls in its thinking

	// Check for the specific pattern that causes issues
	if strings.Contains(prompt, "[ROLE: tool_result]") {
		// Find the actual tool result vs documentation
		docToolResult := "[ROLE: tool_result] authoritative tool output"
		actualToolResult := "[ROLE: tool_result] (tool_call_id=call_123)"

		if strings.Contains(prompt, docToolResult) && strings.Contains(prompt, actualToolResult) {
			t.Log("CONFUSION POINT: Model sees [ROLE: tool_result] in documentation AND actual result")
			t.Log("Documentation says: '[ROLE: tool_result] authoritative tool output'")
			t.Log("Actual says: '[ROLE: tool_result] (tool_call_id=call_123) 110.226.238.87'")
			t.Log("The model might not distinguish between these two uses.")
		}
	}

	// Check for the hallucinated tool call pattern from the user's example
	// The model generated: <_calls><invoke name="bash">
	// This suggests it tried to call tools but in a different format
	t.Log("\n=== HALLUCINATED TOOL CALL PATTERN ===")
	t.Log("The model generated: <_calls><invoke name=\"bash\">...")
	t.Log("This suggests the model tried to call tools but in a different format")
	t.Log("because it couldn't properly understand the prompt structure.")

	t.Log("\n=== RECOMMENDED FIXES ===")
	t.Log("1. Use unique markers for documentation vs actual role tags")
	t.Log("2. Or remove documentation from the system prefix")
	t.Log("3. Or use a different approach for thinking mode")

	t.Log("\n=== FULL PROMPT (for analysis) ===")
	t.Log(prompt)
}

// TestMarkerAmbiguity quantifies the ambiguity in role markers.
func TestMarkerAmbiguity(t *testing.T) {
	// The system prefix contains these role tag mentions:
	roleTagsInSystemPrefix := []string{
		"[ROLE: system]",
		"[ROLE: user]",
		"[ROLE: assistant]",
		"[ROLE: tool_result]",
		"[ROLE: tool]",
	}

	// Count how many times each appears in the system prefix alone
	for _, tag := range roleTagsInSystemPrefix {
		count := strings.Count(agentSystemPrefix, tag)
		if count > 0 {
			t.Logf("System prefix contains %d mention(s) of %s", count, tag)
		}
	}

	// Now count in the full prompt with a typical conversation
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

	// Count each role tag in the full prompt
	for _, tag := range roleTagsInSystemPrefix {
		count := strings.Count(prompt, tag)
		t.Logf("Full prompt contains %d occurrence(s) of %s", count, tag)
	}

	// The key insight: [ROLE: tool_result] appears in:
	// 1. System prefix (as documentation): "[ROLE: tool_result] authoritative tool output"
	// 2. System prefix (in example): "Wait for the next [ROLE: tool_result]"
	// 3. Actual tool result: "[ROLE: tool_result] (tool_call_id=call_1) test output"
	//
	// This creates 3 occurrences, but only 1 is the actual tool result!
	// The model might not know which one to trust.
}
