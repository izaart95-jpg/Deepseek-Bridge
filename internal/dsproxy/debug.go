package dsproxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
)

// Debug mode: enable with --debug or DEBUG=true. When on, the proxy prints
// every HTTP exchange in both directions — client -> proxy and proxy ->
// chat.deepseek.com — including full headers, cookies, request/response
// bodies, SSE frames, PoW challenges/solutions and session IDs.
//
// NOTE: output is unredacted (it is meant for debugging Cloudflare/PoW
// issues), so it contains your DeepSeek token and cookies. Do not share
// debug logs publicly.
const debugBodyLimit = 4096

var debugMode = false

// boolEnv parses truthy env values (1/true/yes/on, case-insensitive).
func boolEnv(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

func debugf(format string, args ...any) {
	if !debugMode {
		return
	}
	fmt.Fprintf(os.Stderr, "\033[96m[DEBUG] "+format+"\033[0m\n", args...)
}

func debugEnabled() bool { return debugMode }

// debugDumpHeaders prints one header block.
func debugDumpHeaders(label string, h http.Header) {
	for k, vv := range h {
		for _, v := range vv {
			debugf("%s %s: %s", label, k, v)
		}
	}
}

func debugTruncate(body []byte) string {
	if len(body) > debugBodyLimit {
		return fmt.Sprintf("%s\n...[truncated %d of %d bytes]", body[:debugBodyLimit], len(body)-debugBodyLimit, len(body))
	}
	return string(body)
}

// debugDumpOutgoingRequest dumps an outgoing HTTP request to DeepSeek.
func debugDumpOutgoingRequest(method, url string, header http.Header, body []byte) {
	debugf("==> %s %s", method, url)
	debugDumpHeaders("  > ", header)
	if len(body) > 0 {
		debugf("  > body: %s", debugIndent(debugTruncate(body)))
	}
}

// debugDumpIncomingResponse dumps a completed upstream response.
func debugDumpIncomingResponse(status int, header http.Header, body []byte) {
	debugf("<== status: %d", status)
	debugDumpHeaders("  < ", header)
	if body != nil {
		debugf("  < body: %s", debugIndent(debugTruncate(body)))
	}
}

// debugDumpClientRequest dumps a client request received by the proxy.
func debugDumpClientRequest(r *http.Request, rawBody []byte) {
	debugf("<-- %s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
	debugDumpHeaders("  <- ", r.Header)
	if rawBody != nil {
		debugf("  <- body: %s", debugIndent(debugTruncate(rawBody)))
	}
}

// debugDumpClientResponse dumps what the proxy sends back to the client.
func debugDumpClientResponse(status int, header http.Header) {
	debugf("--> status: %d", status)
	debugDumpHeaders("  -> ", header)
}

// debugJSON pretty-prints v for debug output; falls back to %v.
func debugJSON(v any) string {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(raw)
}

func debugIndent(s string) string {
	return strings.ReplaceAll(strings.TrimRight(s, "\n"), "\n", "\n         ")
}
