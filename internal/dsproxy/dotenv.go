package dsproxy

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func warnf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "\033[93m"+format+"\033[0m\n", args...)
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// loadDotEnv loads KEY=VALUE pairs from the first .env file found (the
// working directory first, then the directory of the executable) so users can
// just `cp .env.example .env` instead of exporting variables manually.
//
// Rules:
//   - blank lines and lines starting with '#' are skipped;
//   - an optional leading "export " is ignored;
//   - surrounding single/double quotes on values are stripped;
//   - values are taken verbatim otherwise (no inline-comment stripping, so
//     tokens containing '#' stay intact);
//   - variables already present in the environment are never overridden.
//
// It returns the path of the .env file that was applied, or "" if none.
func loadDotEnv() string {
	paths := []string{".env"}
	if exe, err := os.Executable(); err == nil {
		if dir := filepath.Dir(exe); dir != "" {
			paths = append(paths, filepath.Join(dir, ".env"))
		}
	}
	for _, path := range paths {
		if applyDotEnv(path) {
			return path
		}
	}
	return ""
}

// LoadDotEnvAt applies a single .env file explicitly (used by tests and
// available to embedders). It reports whether the file existed.
func LoadDotEnvAt(path string) bool {
	return applyDotEnv(path)
}

func applyDotEnv(path string) bool {
	raw, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') ||
				(value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, value)
		}
	}
	return true
}
