package tests

import (
	"encoding/json"
	"os"
	"testing"

	"deepseek/internal/dsproxy"
)

// TestThinkingRequested pins the contract: thinking turns on only when the
// client payload contains "reasoning": {"enabled": true} or a
// "reasoning_effort" string. Nothing else (model name, legacy flags) enables it.
func TestThinkingRequested(t *testing.T) {
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"plain request stays off", `{"model":"deepseek-chat","messages":[]}`, false},
		{"model name never implies thinking", `{"model":"deepseek-reasoner"}`, false},
		{"legacy thinking flag ignored", `{"thinking":true}`, false},
		{"legacy deepThink flag ignored", `{"deepThink":true}`, false},
		{"reasoning enabled true toggles on", `{"reasoning":{"enabled":true}}`, true},
		{"reasoning enabled false stays off", `{"reasoning":{"enabled":false}}`, false},
		{"reasoning object without enabled stays off", `{"reasoning":{"effort":"high"}}`, false},
		{"any reasoning_effort string toggles on", `{"reasoning_effort":"banana"}`, true},
		{"openai-style reasoning_effort toggles on", `{"reasoning_effort":"high"}`, true},
		{"reasoning_effort null stays off", `{"reasoning_effort":null}`, false},
		{"enabled false but effort present toggles on", `{"reasoning":{"enabled":false},"reasoning_effort":"low"}`, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var req dsproxy.ChatRequest
			if err := json.Unmarshal([]byte(tc.body), &req); err != nil {
				t.Fatalf("unmarshal %s: %v", tc.body, err)
			}
			if got := dsproxy.ThinkingRequested(req); got != tc.want {
				t.Fatalf("ThinkingRequested(%s) = %v, want %v", tc.body, got, tc.want)
			}
		})
	}
}

// TestProxyModelsHasNoReasoner verifies the deepseek-reasoner entry was
// removed from the advertised model list.
func TestProxyModelsHasNoReasoner(t *testing.T) {
	for _, m := range dsproxy.ProxyModels {
		if id, _ := m["id"].(string); id == "deepseek-reasoner" {
			t.Fatal("deepseek-reasoner must be removed from ProxyModels")
		}
	}
}

// TestLoadDotEnv covers the .env loader: values are applied, quoting is
// stripped, comments/export prefixes are handled and existing variables win.
func TestLoadDotEnv(t *testing.T) {
	path := t.TempDir() + "/.env"
	content := "# comment line\n" +
		"TEST_KEY_A=plain-value\n" +
		"export TEST_KEY_B=exported value\n" +
		"TEST_KEY_C=\"quoted value\"\n" +
		"TEST_KEY_D='single'\n" +
		"TEST_HASH=abc#notacomment\n" +
		"\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TEST_KEY_A", "preset-wins")

	if !dsproxy.LoadDotEnvAt(path) {
		t.Fatalf("LoadDotEnvAt(%s) = false, want true", path)
	}
	checks := map[string]string{
		"TEST_KEY_A": "preset-wins",     // existing env must not be overridden
		"TEST_KEY_B": "exported value",  // export prefix handled
		"TEST_KEY_C": "quoted value",    // double quotes stripped
		"TEST_KEY_D": "single",          // single quotes stripped
		"TEST_HASH":  "abc#notacomment", // '#' inside value kept intact
	}
	for k, want := range checks {
		if got := os.Getenv(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
		os.Unsetenv(k)
	}
}
