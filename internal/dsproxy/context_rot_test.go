package dsproxy

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestContextRotHypothesis tests whether the agent mode properly handles
// tool results when reasoning/thinking is enabled.
//
// The hypothesis is that since all messages (system prompt, user messages,
// tool calls, tool results) are combined into a single prompt string in
// agent mode, the model might not properly distinguish between different
// sections, especially tool_results, leading to hallucinated tool calls.
func TestContextRotHypothesis(t *testing.T) {
	// Simulate a conversation where:
	// 1. User asks for public IP
	// 2. Assistant makes a tool call
	// 3. Tool returns the IP
	// 4. User asks a follow-up
	// 5. Model should understand the context properly

	messages := []chatMessage{
		{
			Role:    "system",
			Content: json.RawMessage(`"You are a helpful assistant."`),
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

	// Build the agent prompt
	prompt := buildAgentPrompt(messages, tools)

	// Verify the prompt structure
	t.Log("Generated agent prompt:")
	t.Log(prompt)

	// Check that tool results are properly marked with <<TOOL_RESULT>>
	// (not [ROLE: tool_result] which causes marker ambiguity)
	if !strings.Contains(prompt, "<<TOOL_RESULT>>") {
		t.Error("Prompt should contain <<TOOL_RESULT>> marker")
	}

	// Check that tool_call_id is present
	if !strings.Contains(prompt, "tool_call_id=call_123") {
		t.Error("Prompt should contain tool_call_id")
	}

	// Check that assistant's tool call is rendered
	if !strings.Contains(prompt, "<<<TOOL_CALL>>>") {
		t.Error("Prompt should contain <<<TOOL_CALL>>> from assistant message")
	}

	// Check that the final user message is present
	if !strings.Contains(prompt, "find my os arch") {
		t.Error("Prompt should contain the final user message")
	}

	// The key issue: when reasoning is enabled, does the model understand
	// that the tool_result contains the actual IP address?
	// Let's check if the tool result content is visible in the prompt
	if !strings.Contains(prompt, "110.226.238.87") {
		t.Error("Prompt should contain the tool result content (IP address)")
	}
}

// TestToolResultVisibility checks that tool results are clearly visible
// in the generated prompt, not buried in a way the model might miss.
func TestToolResultVisibility(t *testing.T) {
	messages := []chatMessage{
		{
			Role:    "user",
			Content: json.RawMessage(`"What is my IP?"`),
		},
		{
			Role:    "assistant",
			Content: json.RawMessage(`""`),
			ToolCalls: []assistantToolCall{
				{
					ID:   "call_abc",
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
			Content:    json.RawMessage(`"192.168.1.100"`),
			ToolCallID: "call_abc",
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

	// Verify tool result is clearly marked with <<TOOL_RESULT>> and visible
	lines := strings.Split(prompt, "\n")
	toolResultFound := false
	for _, line := range lines {
		if strings.Contains(line, "<<TOOL_RESULT>>") {
			toolResultFound = true
			// Check the tool result is on the same line or nearby
			if !strings.Contains(line, "192.168.1.100") {
				t.Errorf("Tool result content should be visible near <<TOOL_RESULT>>, got: %s", line)
			}
		}
	}

	if !toolResultFound {
		t.Error("No <<TOOL_RESULT>> found in prompt")
	}

	t.Log("Prompt with tool result:")
	t.Log(prompt)
}

// TestMultipleToolResults verifies that multiple tool results are properly
// distinguished in the prompt.
func TestMultipleToolResults(t *testing.T) {
	messages := []chatMessage{
		{
			Role:    "user",
			Content: json.RawMessage(`"Get my IP and OS"`),
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
				{
					ID:   "call_2",
					Type: "function",
					Function: struct {
						Name      string          `json:"name"`
						Arguments json.RawMessage `json:"arguments"`
					}{
						Name:      "bash",
						Arguments: json.RawMessage(`{"command":"uname -a"}`),
					},
				},
			},
		},
		{
			Role:       "tool",
			Content:    json.RawMessage(`"10.0.0.1"`),
			ToolCallID: "call_1",
		},
		{
			Role:       "tool",
			Content:    json.RawMessage(`"Linux server 5.15.0 x86_64"`),
			ToolCallID: "call_2",
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

	// Verify both tool results are present with correct IDs
	if !strings.Contains(prompt, "tool_call_id=call_1") {
		t.Error("Missing tool_call_id=call_1")
	}
	if !strings.Contains(prompt, "tool_call_id=call_2") {
		t.Error("Missing tool_call_id=call_2")
	}

	// Verify both tool result contents are present
	if !strings.Contains(prompt, "10.0.0.1") {
		t.Error("Missing first tool result content")
	}
	if !strings.Contains(prompt, "Linux server 5.15.0 x86_64") {
		t.Error("Missing second tool result content")
	}

	t.Log("Prompt with multiple tool results:")
	t.Log(prompt)
}

// TestPromptStructureAnalysis analyzes the prompt structure to identify
// potential issues with model understanding.
func TestPromptStructureAnalysis(t *testing.T) {
	messages := []chatMessage{
		{
			Role:    "system",
			Content: json.RawMessage(`"You are a helpful assistant with access to bash."`),
		},
		{
			Role:    "user",
			Content: json.RawMessage(`"Find my public IP"`),
		},
		{
			Role:    "assistant",
			Content: json.RawMessage(`""`),
			ToolCalls: []assistantToolCall{
				{
					ID:   "call_ip",
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
			Content:    json.RawMessage(`"203.0.113.42"`),
			ToolCallID: "call_ip",
		},
		{
			Role:    "assistant",
			Content: json.RawMessage(`"Your public IP is 203.0.113.42."`),
		},
		{
			Role:    "user",
			Content: json.RawMessage(`"Now find my OS architecture"`),
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

	// Analyze the prompt structure
	t.Log("=== PROMPT STRUCTURE ANALYSIS ===")

	// Check for role markers
	roleMarkers := []string{
		"[ROLE: system]",
		"[ROLE: user]",
		"[ROLE: assistant]",
		"[ROLE: tool_result]",
	}

	for _, marker := range roleMarkers {
		count := strings.Count(prompt, marker)
		t.Logf("%s: %d occurrences", marker, count)
	}

	// Check for potential confusion points
	t.Log("\n=== POTENTIAL CONFUSION POINTS ===")

	// 1. Check if tool result is near assistant message
	toolResultIdx := strings.Index(prompt, "<<TOOL_RESULT>>")
	if toolResultIdx >= 0 {
		assistantAfterToolIdx := strings.Index(prompt[toolResultIdx:], "[ROLE: assistant]")
		if assistantAfterToolIdx > 0 && assistantAfterToolIdx < 100 {
			t.Log("WARNING: Tool result is very close to next assistant message")
		}
	}

	// 2. Check for overlapping content
	if strings.Contains(prompt, "203.0.113.42") && strings.Contains(prompt, "find my OS") {
		t.Log("INFO: Both tool result and new user request are in prompt")
	}

	// 3. Check prompt length
	t.Logf("Prompt length: %d characters", len(prompt))

	// 4. Check for clear separation between sections
	lines := strings.Split(prompt, "\n")
	emptyLineCount := 0
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			emptyLineCount++
		}
	}
	t.Logf("Empty lines (potential separators): %d", emptyLineCount)

	t.Log("\n=== FULL PROMPT ===")
	t.Log(prompt)
}
