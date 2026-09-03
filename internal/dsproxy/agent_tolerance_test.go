package dsproxy

import (
	"strings"
	"testing"
)

// ── marker finder ────────────────────────────────────────────────────────────

func TestFindAgentMarkerVariants(t *testing.T) {
	cases := []struct {
		in     string
		wantAt int // -1 = no match
		wantLn int
	}{
		{"<<<TOOL_CALL>>>", 0, 15},     // canonical
		{"<<TOOL_CALL>>>", 0, 14},      // live failure: one '<' short
		{"<<<TOOL_CALL>>", 0, 14},      // one '>' short
		{"<<<<TOOL_CALL>>>>", 0, 17},   // worst tolerated spelling
		{"x <<<TOOL_CALL>>> y", 2, 15}, // embedded
		{"<TOOL_CALL>", -1, 0},         // too few brackets
		{"<<<TOOL_CALL>", -1, 0},       // unterminated
		{"TOOL_CALL", -1, 0},           // bare word
		{"<<<<<<<TOOL_CALL>>>", -1, 0}, // bracket run longer than tolerated
	}
	for _, c := range cases {
		at, ln := findAgentMarker(c.in, agentStartWord, true)
		if at != c.wantAt || (c.wantAt >= 0 && ln != c.wantLn) {
			t.Errorf("findAgentMarker(%q) = (%d,%d), want (%d,%d)", c.in, at, ln, c.wantAt, c.wantLn)
		}
	}
	// The TOOL_CALL inside an END marker must never be taken as a start.
	if at, _ := findAgentMarker(agentToolEnd, agentStartWord, true); at != -1 {
		t.Errorf("END marker matched as start at %d", at)
	}
	if n := bracketRunBack("a<<<b", 4, '<'); n != 3 {
		t.Errorf("bracketRunBack = %d, want 3", n)
	}
	if got := agentWorstMarkerLen; got != len("<<<<TOOL_CALL>>>>") {
		t.Errorf("agentWorstMarkerLen = %d, want %d", got, len("<<<<TOOL_CALL>>>>"))
	}

	// Streaming mode: a trailing '>' run touching the end of the data must
	// wait instead of matching short — this keeps the third '>' of a
	// canonical marker from leaking when chunks split the marker.
	for _, tc := range []struct{ in, word string }{
		{"a <<<TOOL_CALL>>", agentStartWord},
		{"x <<<END_TOOL_CALL>>", agentEndWord},
	} {
		if at, _ := findAgentMarker(tc.in, tc.word, false); at != markerIncomplete {
			t.Errorf("findAgentMarker(%q, final=false) = %d, want markerIncomplete", tc.in, at)
		}
	}
	if at, _ := findAgentMarker("<<<TOOL_CALL>> x", agentStartWord, false); at != 0 {
		t.Errorf("terminated short run should match at 0, got %d", at)
	}
}

// ── finished-text parsing ────────────────────────────────────────────────────

// The exact RESPONSE payload reconstructed from the failing debug session
// (deepseek-v4-pro, "Find my public ip").
const malformedPayload = "<<TOOL_CALL>>>\n{\"name\":\"bash\",\"arguments\":{\"command\":\"curl -s https://api.ipify.org\"}}\n<<<END_TOOL_CALL>>>"

func TestParseAgentToolCallsTolerantMarkers(t *testing.T) {
	cases := []struct {
		text     string
		wantArgs string // substring expected inside arguments JSON
	}{
		{malformedPayload, "api.ipify.org"},
		{"Let me check.\n" + malformedPayload + "\n", "api.ipify.org"},
		{"<<<<TOOL_CALL>>>>\n{\"name\":\"bash\",\"arguments\":{}}\n<<<<END_TOOL_CALL>>>>", "{}"},
	}
	for _, c := range cases {
		calls := ParseAgentToolCalls(c.text)
		if len(calls) != 1 {
			t.Fatalf("ParseAgentToolCalls(%q...) => %d calls, want 1", c.text[:12], len(calls))
		}
		fn := calls[0]["function"].(map[string]any)
		if fn["name"] != "bash" {
			t.Errorf("name = %v, want bash", fn["name"])
		}
		args := fn["arguments"].(string)
		if !strings.Contains(args, c.wantArgs) {
			t.Errorf("arguments = %s, want substring %s", args, c.wantArgs)
		}
		if stripped := StripAgentToolCalls(c.text); strings.Contains(stripped, "TOOL_CALL") {
			t.Errorf("StripAgentToolCalls left markers: %q", stripped)
		}
	}
}

func TestNormalizeAgentFencesTolerantMarkers(t *testing.T) {
	in := "```json\n<<TOOL_CALL>>>\n{\"name\":\"bash\",\"arguments\":{}}\n<<<END_TOOL_CALL>>>\n```\ndone"
	got := NormalizeAgentFences(in)
	want := "<<TOOL_CALL>>>\n{\"name\":\"bash\",\"arguments\":{}}\n<<<END_TOOL_CALL>>>\ndone"
	if got != want {
		t.Errorf("normalize:\n got %q\nwant %q", got, want)
	}
}

// ── streaming interceptor ────────────────────────────────────────────────────

// Replays the RESPONSE fragment chunks exactly as they arrived in the failing
// session's upstream SSE log, byte for byte.
var debugFragmentChunks = []string{
	"<<", "<", "TO", "OL", "_C", "ALL", ">>", ">\n",
	"{\"", "name", "\":\"", "bash", "\",\"",
	"arguments", "\":{\"", "command", "\":\"", "curl",
	" -", "s", " https", "://", "api", ".ip", "ify", ".org", "\"", "}}\n",
	"<<", "<", "END", "_TO", "OL", "_C", "ALL", ">>>",
}

func feedChunks(t *testing.T, chunks []string, step int) (content string, toolCalls int, args string) {
	t.Helper()
	in := &AgentStreamInterceptor{}
	var b strings.Builder
	for i := 0; i < len(chunks); i += step {
		piece := ""
		for j := i; j < i+step && j < len(chunks); j++ {
			piece += chunks[j]
		}
		parsed := in.Feed(piece)
		b.WriteString(parsed.Content)
		for _, call := range parsed.ToolCalls {
			fn := call["function"].(map[string]any)
			args, _ = fn["arguments"].(string)
			toolCalls++
		}
	}
	final := in.Finish()
	for _, call := range final.ToolCalls {
		fn := call["function"].(map[string]any)
		args, _ = fn["arguments"].(string)
		toolCalls++
	}
	return b.String() + final.Content, toolCalls, args
}

func TestStreamInterceptorReplaysMalformedDebugStream(t *testing.T) {
	for _, step := range []int{1, 3, 7} { // one chunk / log-like bursts / coarse
		content, calls, args := feedChunks(t, debugFragmentChunks, step)
		if calls != 1 {
			t.Errorf("step=%d: %d tool calls, want 1 (content=%q)", step, calls, content)
		}
		if !strings.Contains(args, "api.ipify.org") {
			t.Errorf("step=%d: arguments %q missing ipify command", step, args)
		}
		if strings.Contains(content, "TOOL_CALL") || strings.TrimSpace(content) != "" {
			t.Errorf("step=%d: markers or junk leaked as content: %q", step, content)
		}
	}
}

func TestStreamInterceptorShortTailVariant(t *testing.T) {
	stream := "checking…\n```json\n<<<TOOL_CALL>>\n{\"name\":\"bash\",\"arguments\":{\"command\":\"uname\"}}\n<<<END_TOOL_CALL>>>\n```\n"
	content, calls, _ := feedChunks(t, splitRunes(stream, 3), 1)
	if calls != 1 {
		t.Errorf("%d tool calls, want 1", calls)
	}
	if strings.Contains(content, "```") || strings.Contains(content, "TOOL_CALL") {
		t.Errorf("leaked: %q", content)
	}
	if !strings.Contains(content, "checking") {
		t.Errorf("prose damaged: %q", content)
	}
}

// An unterminated opening marker must not hang the stream nor be parsed.
func TestStreamInterceptorUnterminatedMarkerStaysContent(t *testing.T) {
	in := &AgentStreamInterceptor{}
	parsed := in.Feed("<TOOL_CALL> just talking about tools")
	final := in.Finish()
	out := parsed.Content + final.Content
	if len(parsed.ToolCalls)+len(final.ToolCalls) != 0 {
		t.Errorf("single-bracket text produced tool calls")
	}
	if out != "<TOOL_CALL> just talking about tools" {
		t.Errorf("single-bracket text altered: %q", out)
	}
}

// Invalid JSON inside a recognized block stays visible text (existing policy).
func TestStreamInvalidBlockLeaksAsContent(t *testing.T) {
	content, calls, _ := feedChunks(t, splitRunes("<<TOOL_CALL>>>\nnot json\n<<<END_TOOL_CALL>>>", 4), 1)
	if calls != 0 {
		t.Errorf("%d calls, want 0 for invalid block", calls)
	}
	if !strings.Contains(content, "not json") {
		t.Errorf("invalid block vanished instead of leaking: %q", content)
	}
}

func TestStreamTwoSequentialBlocks(t *testing.T) {
	stream := "<<TOOL_CALL>>>\n{\"name\":\"a\",\"arguments\":{}}\n<<<END_TOOL_CALL>>>\n<<TOOL_CALL>>>\n{\"name\":\"b\",\"arguments\":{}}\n<<<END_TOOL_CALL>>>"
	in := &AgentStreamInterceptor{}
	names := []string{}
	collect := func(parsed AgentParsedChunk) {
		for _, call := range parsed.ToolCalls {
			names = append(names, call["function"].(map[string]any)["name"].(string))
		}
	}
	for i := 0; i < len(stream); i += 5 {
		end := i + 5
		if end > len(stream) {
			end = len(stream)
		}
		collect(in.Feed(stream[i:end]))
	}
	collect(in.Finish())
	final := in.Finish()
	if strings.Contains(final.Content, "TOOL_CALL") {
		t.Errorf("trailing markers flushed: %q", final.Content)
	}
	if len(names) != 2 || names[0] != "a" || names[1] != "b" {
		t.Errorf("names = %v, want [a b]", names)
	}
}

// splitRunes cuts s into pieces of n bytes (ASCII input assumed).
func splitRunes(s string, n int) []string {
	var out []string
	for i := 0; i < len(s); i += n {
		end := i + n
		if end > len(s) {
			end = len(s)
		}
		out = append(out, s[i:end])
	}
	return out
}
