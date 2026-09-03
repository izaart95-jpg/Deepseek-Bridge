package main

import (
	"strings"
	"testing"
)

func TestTrimTrailingAgentFence(t *testing.T) {
	cases := []struct{ in, want string }{
		{"I'll check.\n```json", "I'll check."},
		{"I'll check.\n```", "I'll check."},
		{"plain", "plain"},
		{"hello ```bash\nx=1", "hello ```bash\nx=1"}, // real code block untouched
	}
	for _, c := range cases {
		if got := trimTrailingAgentFence(c.in); got != c.want {
			t.Errorf("trim(%q)=%q want %q", c.in, got, c.want)
		}
	}
}

func TestSkipLeadingAgentFence(t *testing.T) {
	if n := skipLeadingAgentFence("```\nrest"); n != 4 {
		t.Errorf("bare fence: got %d", n)
	}
	if n := skipLeadingAgentFence("```json\ntext"); n != 8 {
		t.Errorf("json fence: got %d", n)
	}
	if n := skipLeadingAgentFence("```python\nx"); n != 0 {
		t.Errorf("lang fence must not match: got %d", n)
	}
	if n := skipLeadingAgentFence("text"); n != 0 {
		t.Errorf("text: got %d", n)
	}
}

func TestNormalizeAgentFences(t *testing.T) {
	in := "```json\n<<<TOOL_CALL>>>\n{\"name\":\"bash\",\"arguments\":{}}\n<<<END_TOOL_CALL>>>\n```\ndone"
	want := "<<<TOOL_CALL>>>\n{\"name\":\"bash\",\"arguments\":{}}\n<<<END_TOOL_CALL>>>\ndone"
	if got := normalizeAgentFences(in); got != want {
		t.Errorf("normalize:\n got %q\nwant %q", got, want)
	}
}

func TestInterceptorSwallowsFencesAcrossChunks(t *testing.T) {
	stream := "Let me check your architecture.\n\n```json\n<<<TOOL_CALL>>>\n{\"name\":\"bash\",\"arguments\":{\"command\":\"uname -m\"}}\n<<<END_TOOL_CALL>>>\n```\ntrailing note"
	in := &agentStreamInterceptor{}
	var content strings.Builder
	var calls int
	for i := 0; i < len(stream); i += 3 { // nasty 3-byte chunking
		end := i + 3
		if end > len(stream) {
			end = len(stream)
		}
		parsed := in.feed(stream[i:end])
		content.WriteString(parsed.content)
		calls += len(parsed.toolCalls)
	}
	content.WriteString(in.flush())
	out := content.String()
	if calls != 1 {
		t.Errorf("expected 1 tool call, got %d", calls)
	}
	if strings.Contains(out, "```") || strings.Contains(out, "json\n<<<") {
		t.Errorf("fence leaked into content: %q", out)
	}
	if !strings.Contains(out, "Let me check") || !strings.Contains(out, "trailing note") {
		t.Errorf("prose damaged: %q", out)
	}
}

func TestNonStreamParseStripWithFences(t *testing.T) {
	text := "Sure!\n```json\n<<<TOOL_CALL>>>\n{\"name\":\"bash\",\"arguments\":{\"command\":\"arch\"}}\n<<<END_TOOL_CALL>>>\n```\nRunning now."
	calls := parseAgentToolCalls(text)
	if len(calls) != 1 || calls[0]["function"].(map[string]any)["name"] != "bash" {
		t.Fatalf("parse failed: %#v", calls)
	}
	if stripped := stripAgentToolCalls(text); strings.Contains(stripped, "```") || strings.Contains(stripped, "TOOL_CALL") {
		t.Errorf("strip left junk: %q", stripped)
	}
}
