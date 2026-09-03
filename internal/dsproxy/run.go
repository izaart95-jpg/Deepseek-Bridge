package dsproxy

import (
	"log"
	"net/http"
	"os"
)

// Run loads .env (real environment variables always win), applies the
// --debug/--agent-mode flags and DEBUG/AGENT_MODE variables, then starts the
// OpenAI-compatible proxy server.
func Run() {
	envFile := loadDotEnv()

	debugMode = false
	agentMode = false
	for _, arg := range os.Args[1:] {
		switch arg {
		case "--debug":
			debugMode = true
		case "--agent-mode":
			agentMode = true
		}
	}
	if boolEnv("DEBUG") {
		debugMode = true
	}
	if boolEnv("AGENT_MODE") {
		agentMode = true
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
	logger.Printf("  Endpoints : POST /v1/chat/completions")
	logger.Printf("              GET  /v1/models")
	logger.Printf("              GET  /history?enable=true|false")
	logger.Printf("              POST /new")

	server := &http.Server{
		Addr:    "0.0.0.0:" + port,
		Handler: NewProxyServer(logger, proxyKey),
	}
	logger.Printf("Ready -> http://0.0.0.0:%s/v1", port)
	if err := server.ListenAndServe(); err != nil {
		logger.Fatalf("server error: %v", err)
	}
}
