package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"os"
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

func runProxy() {
	port := envOr("PORT", "3000")
	proxyKey := envOr("PROXY_API_KEY", "Waguri-san")
	token := os.Getenv("DEEPSEEK_TOKEN")

	logger := log.New(os.Stdout, "", log.LstdFlags)
	logger.Printf("DeepSeek OpenAI Proxy  v1 (Go port)")
	logger.Printf("  Port      : %s", port)
	logger.Printf("  Proxy key : %s", proxyKey)
	if token == "" {
		logger.Printf("  DS token  : NOT SET  <- export DEEPSEEK_TOKEN=...")
	} else {
		logger.Printf("  DS token  : SET")
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

func main() {
	runProxy()
}
