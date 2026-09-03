package dsproxy

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// Run loads .env (real environment variables always win), applies the
// --debug/--agent-mode/--sync-mode flags and DEBUG/AGENT_MODE/SYNC_MODE
// variables, then starts the OpenAI-compatible proxy server.
//
// By default stateless (history=false) requests run through the async
// session-pool flow: a standing batch of ready sessions is pre-made at
// startup, handed out instantly, and refilled as requests complete.
// --sync-mode restores the legacy synchronous flow where every request
// creates its own session first.
func Run() {
	envFile := loadDotEnv()

	debugMode = false
	agentMode = false
	syncMode := false
	for _, arg := range os.Args[1:] {
		switch arg {
		case "--debug":
			debugMode = true
		case "--agent-mode":
			agentMode = true
		case "--sync-mode":
			syncMode = true
		}
	}
	if boolEnv("DEBUG") {
		debugMode = true
	}
	if boolEnv("AGENT_MODE") {
		agentMode = true
	}
	if boolEnv("SYNC_MODE") {
		syncMode = true
	}

	port := envOr("PORT", "3000")
	proxyKey := envOr("PROXY_API_KEY", "Waguri")
	token := os.Getenv("DEEPSEEK_TOKEN")

	logger := log.New(os.Stdout, "", log.LstdFlags)
	logger.Printf("DeepSeek OpenAI Proxy  v1 (Go port)")
	logger.Printf("  Port      : %s", port)
	logger.Printf("  Proxy key : %s", proxyKey)
	if envFile != "" {
		logger.Printf("  Env file  : %s", envFile)
	}
	if token == "" {
		logger.Printf("  DS token  : NOT SET  <- put DEEPSEEK_TOKEN=... in .env or export it")
	} else {
		logger.Printf("  DS token  : SET")
	}
	logger.Printf("  Debug     : %v", debugMode)
	if agentMode {
		logger.Printf("  Agent mode: ENABLED (OpenAI tools/roles translated, tool calls intercepted)")
	} else {
		logger.Printf("  Agent mode: disabled")
	}

	proxy := NewProxyServer(logger, proxyKey)

	var pool *SessionPool
	if syncMode {
		logger.Printf("  Mode      : sync (--sync-mode: one session created per request)")
	} else {
		poolSize := intEnvOr("SESSION_POOL_SIZE", defaultPoolSize)
		waitSecs := intEnvOr("SESSION_ACQUIRE_TIMEOUT", int(defaultPoolWait/time.Second))
		proxy.poolWait = time.Duration(waitSecs) * time.Second
		if waitSecs <= 0 {
			proxy.poolWait = 0 // 0 => wait indefinitely for a pooled session
		}
		pool = NewSessionPool(logger, &lazyBackend{s: proxy}, poolSize)
		proxy.AttachSessionPool(pool)
		logger.Printf("  Mode      : async (pre-made session batch x%d, history=false only)", pool.Size())
		logger.Printf("              SESSION_POOL_SIZE=%d SESSION_ACQUIRE_TIMEOUT=%ds", pool.Size(), waitSecs)
	}

	logger.Printf("  Endpoints : POST /v1/chat/completions")
	logger.Printf("              GET  /v1/models")
	logger.Printf("              GET  /history?enable=true|false")
	logger.Printf("              POST /new")

	server := &http.Server{
		Addr:    "0.0.0.0:" + port,
		Handler: proxy,
	}

	// Start serving before blocking on signals.
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.ListenAndServe()
	}()

	// Warm the standing session batch in the background; requests are served
	// meanwhile (they simply queue on Acquire until sessions appear).
	if pool != nil {
		pool.Start()
	}

	logger.Printf("Ready -> http://0.0.0.0:%s/v1", port)

	ctx, stopSignal := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignal()

	select {
	case err := <-serveErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatalf("server error: %v", err)
		}
	case <-ctx.Done():
		// Respectful stop (CTRL+C / SIGTERM). Re-arm default signal handling
		// so a second interrupt force-exits immediately.
		stopSignal()
		logger.Printf("Shutdown requested — draining connections and clearing all sessions...")

		drainCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		if err := server.Shutdown(drainCtx); err != nil {
			logger.Printf("drain deadline hit (%v); closing remaining connections", err)
			_ = server.Close()
		}
		cancel()

		// Clear any sessions still pooled so nothing is left behind on the
		// DeepSeek account (checked-out ones are deleted by their own Release).
		if pool != nil {
			pool.Shutdown()
		}
		logger.Printf("All sessions cleared. Goodbye.")
	}
}
