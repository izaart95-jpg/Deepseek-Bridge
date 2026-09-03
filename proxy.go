package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

var proxyModels = []map[string]any{
	{"id": "deepseek-chat", "object": "model", "created": 1700000000, "owned_by": "deepseek"},
	{"id": "deepseek-reasoner", "object": "model", "created": 1700000000, "owned_by": "deepseek"},
}

// ProxyServer is the OpenAI-compatible proxy (port of proxy.py).
type ProxyServer struct {
	log      *log.Logger
	proxyKey string

	mu     sync.Mutex
	api    *DeepSeekAPI
	apiErr error
	apiSet bool

	hl         sync.Mutex
	histChatID string
	histParID  any
	useHistory bool
}

func NewProxyServer(logger *log.Logger, proxyKey string) *ProxyServer {
	return &ProxyServer{log: logger, proxyKey: proxyKey}
}

func (s *ProxyServer) getAPI() (*DeepSeekAPI, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.apiSet {
		return s.api, s.apiErr
	}
	token := os.Getenv("DEEPSEEK_TOKEN")
	if token == "" {
		s.apiErr = errors.New("DEEPSEEK_TOKEN not set.\n  export DEEPSEEK_TOKEN=<chat.deepseek.com localStorage -> userToken -> value>")
	} else {
		s.api, s.apiErr = NewDeepSeekAPI(token)
		if s.apiErr == nil {
			s.log.Printf("DeepSeekAPI ready")
		}
	}
	s.apiSet = true
	return s.api, s.apiErr
}

func (s *ProxyServer) newHist() (string, error) {
	api, err := s.getAPI()
	if err != nil {
		return "", err
	}
	cid, err := api.CreateChatSession(context.Background())
	if err != nil {
		return "", err
	}
	s.hl.Lock()
	s.histChatID, s.histParID = cid, nil
	s.hl.Unlock()
	s.log.Printf("New history session: %s", cid)
	return cid, nil
}

func (s *ProxyServer) getHist() (string, any) {
	s.hl.Lock()
	defer s.hl.Unlock()
	return s.histChatID, s.histParID
}

func (s *ProxyServer) setHistPar(mid any) {
	s.hl.Lock()
	defer s.hl.Unlock()
	s.histParID = mid
}

func (s *ProxyServer) setUseHistory(v bool) {
	s.hl.Lock()
	defer s.hl.Unlock()
	s.useHistory = v
}

func (s *ProxyServer) getUseHistory() bool {
	s.hl.Lock()
	defer s.hl.Unlock()
	return s.useHistory
}

func (s *ProxyServer) checkAuth(r *http.Request) bool {
	if s.proxyKey == "" {
		return true
	}
	return r.Header.Get("Authorization") == "Bearer "+s.proxyKey
}

func (s *ProxyServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	cors := func() {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	}

	if r.Method == http.MethodOptions {
		cors()
		w.WriteHeader(200)
		return
	}
	cors()

	if !s.checkAuth(r) {
		writeError(w, "Bad token", "authentication_error", "invalid_api_key", 401)
		return
	}

	path := r.URL.Path
	if r.Method == http.MethodGet {
		switch path {
		case "/v1/models", "/models":
			writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": proxyModels})
			return
		case "/history":
			q := r.URL.Query()
			v := q.Get("enable")
			if v == "" {
				v = q.Get("value")
			}
			enabled := strings.EqualFold(v, "true")
			s.setUseHistory(enabled)
			cid, _ := s.getHist()
			writeJSON(w, http.StatusOK, map[string]any{"use_history": enabled, "chat_id": cid})
			return
		}
		writeError(w, "Not Found", "invalid_request_error", "not_found", 404)
		return
	}

	if r.Method == http.MethodPost {
		switch path {
		case "/new":
			if _, err := s.getAPI(); err != nil {
				writeError(w, err.Error(), "authentication_error", "missing_token", 401)
				return
			}
			cid, err := s.newHist()
			if err != nil {
				writeError(w, err.Error(), "upstream_error", nil, 502)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"message": "New session", "chat_id": cid})
			return
		case "/history":
			var body map[string]any
			if err := decodeBody(w, r, &body); err != nil {
				return
			}
			var enabled bool
			if v, ok := body["enable"]; ok {
				enabled = toBool(v)
			} else if v, ok := body["value"]; ok {
				enabled = toBool(v)
			}
			s.setUseHistory(enabled)
			writeJSON(w, http.StatusOK, map[string]any{"use_history": enabled})
			return
		case "/v1/chat/completions", "/chat/completions":
			s.handleChat(w, r)
			return
		}
		writeError(w, "Not Found", "invalid_request_error", "not_found", 404)
		return
	}

	writeError(w, "Not Found", "invalid_request_error", "not_found", 404)
}

// chatRequest mirrors the OpenAI-style request body.
type chatRequest struct {
	Messages []struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	} `json:"messages"`
	Model     string `json:"model"`
	Stream    bool   `json:"stream"`
	Thinking  bool   `json:"thinking"`
	DeepThink bool   `json:"deepThink"`
	Search    bool   `json:"search"`
}

func (s *ProxyServer) handleChat(w http.ResponseWriter, r *http.Request) {
	var body chatRequest
	if err := decodeBody(w, r, &body); err != nil {
		return
	}

	model := body.Model
	if model == "" {
		model = "deepseek-chat"
	}
	stream := body.Stream
	thinking := body.Thinking || body.DeepThink || model == "deepseek-reasoner"
	search := body.Search

	prompt := ""
	if len(body.Messages) > 0 {
		raw := body.Messages[len(body.Messages)-1].Content
		var s string
		if json.Unmarshal(raw, &s) == nil {
			prompt = s
		} else {
			var parts []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}
			if json.Unmarshal(raw, &parts) == nil {
				texts := []string{}
				for _, p := range parts {
					if p.Type == "text" {
						texts = append(texts, p.Text)
					}
				}
				prompt = strings.Join(texts, "\n")
			}
		}
	}
	if prompt == "" {
		prompt = " "
	}

	api, err := s.getAPI()
	if err != nil {
		writeError(w, err.Error(), "authentication_error", "missing_token", 401)
		return
	}

	var chatID string
	var parID any
	if s.getUseHistory() {
		chatID, parID = s.getHist()
		if chatID == "" {
			chatID, err = s.newHist()
			parID = nil
			if err != nil {
				writeError(w, err.Error(), "upstream_error", nil, 502)
				return
			}
		}
	} else {
		chatID, err = api.CreateChatSession(context.Background())
		parID = nil
		if err != nil {
			writeError(w, err.Error(), "upstream_error", nil, 502)
			return
		}
		s.log.Printf("Stateless session: %s", chatID)
	}

	s.log.Printf("-> model=%-20s think=%-5v search=%-5v history=%v", model, thinking, search, s.getUseHistory())

	params := ChatParams{
		ChatSessionID:   chatID,
		Prompt:          prompt,
		ParentMessageID: parID,
		ThinkingEnabled: thinking,
		SearchEnabled:   search,
	}

	if stream {
		s.streamResponse(w, r, api, params, model)
	} else {
		s.blockResponse(w, r, api, params, model)
	}
}

func (s *ProxyServer) streamResponse(w http.ResponseWriter, r *http.Request, api *DeepSeekAPI, params ChatParams, model string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, "Streaming unsupported", "server_error", nil, 500)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "close")
	w.WriteHeader(200)

	rid := "chatcmpl-" + randomHex(16)
	created := time.Now().Unix()

	sse := func(obj map[string]any) bool {
		raw, err := json.Marshal(obj)
		if err != nil {
			return false
		}
		if _, err := w.Write(append([]byte("data: "), append(raw, []byte("\n\n")...)...)); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}

	// role-establishing chunk (required by most clients)
	sse(map[string]any{
		"id": rid, "object": "chat.completion.chunk", "created": created, "model": model,
		"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"role": "assistant", "content": ""}, "finish_reason": nil}},
	})

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	err := api.ChatCompletion(ctx, params, func(ch Chunk) bool {
		if ch.Type == "ready" && s.getUseHistory() && ch.ResponseMessageID != nil {
			s.setHistPar(ch.ResponseMessageID)
		}
		if ch.Type != "content" || ch.Content == "" {
			return false
		}
		return !sse(map[string]any{
			"id": rid, "object": "chat.completion.chunk", "created": created, "model": model,
			"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"content": ch.Content}, "finish_reason": nil}},
		})
	})

	if err != nil {
		etype, _ := errorType(err)
		sse(map[string]any{"error": map[string]any{"message": err.Error(), "type": etype}})
		return
	}

	sse(map[string]any{
		"id": rid, "object": "chat.completion.chunk", "created": created, "model": model,
		"choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": "stop"}},
	})
	w.Write([]byte("data: [DONE]\n\n"))
	flusher.Flush()
}

func (s *ProxyServer) blockResponse(w http.ResponseWriter, r *http.Request, api *DeepSeekAPI, params ChatParams, model string) {
	ctx := r.Context()
	var parts []string

	err := api.ChatCompletion(ctx, params, func(ch Chunk) bool {
		if ch.Type == "ready" && s.getUseHistory() && ch.ResponseMessageID != nil {
			s.setHistPar(ch.ResponseMessageID)
		}
		if ch.Type == "content" && ch.Content != "" {
			parts = append(parts, ch.Content)
		}
		return false
	})

	if err != nil {
		etype, status := errorType(err)
		writeError(w, err.Error(), etype, nil, status)
		return
	}

	answer := strings.Join(parts, "")
	promptTokens := countWords(params.Prompt)
	completionTokens := countWords(answer)

	writeJSON(w, http.StatusOK, map[string]any{
		"id":      "chatcmpl-" + randomHex(16),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []any{map[string]any{
			"index":         0,
			"message":       map[string]any{"role": "assistant", "content": answer},
			"finish_reason": "stop",
		}},
		"usage": map[string]any{
			"prompt_tokens":     promptTokens,
			"completion_tokens": completionTokens,
			"total_tokens":      promptTokens + completionTokens,
		},
	})
}

// ── helpers ──────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, data any) {
	raw, err := json.Marshal(data)
	if err != nil {
		writeError(w, "Internal error", "server_error", nil, 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(raw)
}

func writeError(w http.ResponseWriter, msg, etype string, code any, status int) {
	writeJSON(w, status, map[string]any{
		"error": map[string]any{
			"message": msg,
			"type":    etype,
			"param":   nil,
			"code":    code,
		},
	})
}

func decodeBody(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<20)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(dst); err != nil {
		writeError(w, fmt.Sprintf("Invalid JSON body: %v", err), "invalid_request_error", "invalid_json", 400)
		return err
	}
	return nil
}

func toBool(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		b, _ := strconv.ParseBool(t)
		return b
	}
	return false
}

func countWords(s string) int {
	return len(strings.Fields(s))
}
