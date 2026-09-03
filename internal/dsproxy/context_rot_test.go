package dsproxy

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestContextRotHypothesis tests whether the agent mode properly handles
// tool results when reasoning/thinking is enabled.
//
// The new prompt structure uses XML-like section tags (<tool_result>,
// <tool_exchange>, <current_task>) instead of [ROLE: ...] tags, which
// eliminates marker ambiguity and gives the model clear structure.
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

	// Check that tool results use the new <tool_result> XML tag
	if !strings.Contains(prompt, "<tool_result") {
		t.Error("Prompt should contain <tool_result> tag")
	}

	// Check that call_id is present in the tool_result tag
	if !strings.Contains(prompt, `call_id="call_123"`) {
		t.Error("Prompt should contain call_id attribute in tool_result")
	}

	// Check that assistant's tool call is rendered
	if !strings.Contains(prompt, "<<<TOOL_CALL>>>") {
		t.Error("Prompt should contain <<<TOOL_CALL>>> from assistant message")
	}

	// Check that the current_task section contains the last user message
	if !strings.Contains(prompt, "<current_task>") {
		t.Error("Prompt should have <current_task> section")
	}
	if !strings.Contains(prompt, "find my os arch") {
		t.Error("Prompt should contain the final user message in <current_task>")
	}

	// The key improvement: tool results are in <tool_result> tags, clearly
	// separated from assistant messages in <tool_exchange> blocks
	if !strings.Contains(prompt, "<tool_exchange>") {
		t.Error("Prompt should group tool calls with results in <tool_exchange>")
	}

	// Check that the tool result content is visible in the prompt
	if !strings.Contains(prompt, "110.226.238.87") {
		t.Error("Prompt should contain the tool result content (IP address)")
	}

	// Verify the new structure: no [ROLE: ...] tags
	roleMarkers := []string{"[ROLE: system]", "[ROLE: user]", "[ROLE: assistant]"}
	for _, marker := range roleMarkers {
		if strings.Contains(prompt, marker) {
			t.Errorf("Prompt should not contain old %s marker", marker)
		}
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

	// Verify tool result is in a <tool_result> tag and content is visible
	if !strings.Contains(prompt, "<tool_result") {
		t.Error("No <tool_result> tag found in prompt")
	}
	if !strings.Contains(prompt, "192.168.1.100") {
		t.Error("Tool result content (IP) not visible in prompt")
	}

	// Verify the tool exchange grouping
	if !strings.Contains(prompt, "<tool_exchange>") {
		t.Error("Tool calls and results should be grouped in <tool_exchange>")
	}

	t.Log("Prompt with tool result:")
	t.Log(prompt)
}

// TestMultipleToolResults verifies that multiple tool results are properly
// distinguished in the prompt with call_id attributes.
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

	// Verify both tool results are present with correct call_id attributes
	if !strings.Contains(prompt, `call_id="call_1"`) {
		t.Error("Missing call_id=\"call_1\" in tool_result tag")
	}
	if !strings.Contains(prompt, `call_id="call_2"`) {
		t.Error("Missing call_id=\"call_2\" in tool_result tag")
	}

	// Verify both tool result contents are present
	if !strings.Contains(prompt, "10.0.0.1") {
		t.Error("Missing first tool result content")
	}
	if !strings.Contains(prompt, "Linux server 5.15.0 x86_64") {
		t.Error("Missing second tool result content")
	}

	// Verify tool exchange grouping
	if !strings.Contains(prompt, "<tool_exchange>") {
		t.Error("Tool calls should be grouped in <tool_exchange>")
	}

	t.Log("Prompt with multiple tool results:")
	t.Log(prompt)
}

// TestPromptStructureAnalysis analyzes the new prompt structure and verifies
// it addresses the context rot issues.
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

	// Analyze the NEW prompt structure
	t.Log("=== NEW PROMPT STRUCTURE ANALYSIS ===")

	// Verify the new XML-like section tags are present
	sections := []string{
		"<system>",
		"<tools>",
		"<recent>",
		"<current_task>",
		"<output_rules>",
	}
	for _, section := range sections {
		if !strings.Contains(prompt, section) {
			t.Errorf("Prompt missing section: %s", section)
		}
	}

	// Verify tool exchange grouping
	if !strings.Contains(prompt, "<tool_exchange>") {
		t.Error("Missing <tool_exchange> for grouped tool calls")
	}
	if !strings.Contains(prompt, "</tool_exchange>") {
		t.Error("Missing closing </tool_exchange>")
	}

	// Verify tool_result has call_id
	if !strings.Contains(prompt, `<tool_result call_id="call_ip"`) {
		t.Error("Missing <tool_result> with call_id")
	}

	// Check for OLD format markers (should NOT be present)
	oldMarkers := []string{
		"[ROLE: system]",
		"[ROLE: user]",
		"[ROLE: assistant]",
		"[ROLE: tool_result]",
		"<<TOOL_RESULT>>",
		"[TOOL CONTRACT]",
	}
	for _, marker := range oldMarkers {
		if strings.Contains(prompt, marker) {
			t.Errorf("Prompt should NOT contain old format marker: %s", marker)
		}
	}

	// Check prompt length
	t.Logf("Prompt length: %d characters", len(prompt))

	// Check for clear separation between sections
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
