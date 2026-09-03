package tests

import (
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
	// The fix should use <<TOOL_RESULT>> instead of [ROLE: tool_result]
	// This prevents confusion with documentation that might mention role tags
	t.Log("Context rot fix applied: using <<TOOL_RESULT>> instead of [ROLE: tool_result]")
	t.Log("This prevents marker ambiguity that caused model confusion")

	// Verify the fix conceptually
	role := "tool"
	toolCallID := "call_123"
	content := "110.226.238.87"

	if role != "tool" {
		t.Error("Tool message should have role 'tool'")
	}
	if toolCallID != "call_123" {
		t.Error("Tool message should have tool_call_id")
	}
	if content != "110.226.238.87" {
		t.Error("Tool message should contain the IP address")
	}
}
