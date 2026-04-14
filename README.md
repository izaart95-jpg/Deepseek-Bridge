# DeepSeek Free API

> A Python proxy for DeepSeek with Cloudflare bypass and OpenAI-compatible interface — no paid API key required.

![Python](https://img.shields.io/badge/Python-3.x-blue?style=flat-square)
![OpenAI Compatible](https://img.shields.io/badge/OpenAI-Compatible-green?style=flat-square)
![Cloudflare Bypass](https://img.shields.io/badge/Cloudflare-Bypass-purple?style=flat-square)
![Free API](https://img.shields.io/badge/API-Free-orange?style=flat-square)

---

## Features

- Full DeepSeek API implementation
- Cloudflare protection bypass
- Proof of Work (PoW) challenge solving
- OpenAI-compatible proxy server
- Interactive CLI chat client
- Cookie management system
- Streaming and non-streaming responses
- Threaded conversation support

---

## Installation

### 1. Clone the repository

```bash
git clone https://github.com/izaart95-jpg/DeepRouter.git
cd DeepRouter
```

### 2. Install dependencies

```bash
pip install -r requirements.txt
# or, to ensure latest versions:
pip install --upgrade -r requirements.txt
```

### 3. Obtain your DeepSeek token

1. Navigate to [chat.deepseek.com](https://chat.deepseek.com) and sign in
2. Open browser DevTools (F12) and go to the Console tab
3. Run the following snippet:

```js
JSON.parse(localStorage.getItem("userToken")).value
```

### 4. Configure environment

```bash
cp .env.example .env
```

Edit `.env` with your token, or export it directly:

```bash
# Linux / macOS
export DEEPSEEK_TOKEN=your_token_here

# Windows (PowerShell)
$env:DEEPSEEK_TOKEN="your_token_here"
```

---

## Running

### Interactive CLI chat

```bash
python interactive_chat.py
```

### OpenAI-compatible proxy server

```bash
python proxy.py
```

> **Note:** Best results with Python below 3.12. Compatibility issues may occur on newer versions.

---

## API Reference

The proxy runs at `http://localhost:3000`. All endpoints require the bearer token `deepseek-proxy`.

### `POST /history` — Toggle conversation history

```bash
# Enable
curl -X POST http://localhost:3000/history \
  -H "Authorization: Bearer deepseek-proxy" \
  -H "Content-Type: application/json" \
  -d '{"enable": true}'

# Disable
curl -X POST http://localhost:3000/history \
  -H "Authorization: Bearer deepseek-proxy" \
  -H "Content-Type: application/json" \
  -d '{"enable": false}'
```

### `POST /new` — Create a new session

```bash
curl -X POST http://localhost:3000/new \
  -H "Authorization: Bearer deepseek-proxy"
```

### `POST /v1/chat/completions` — Chat completions (OpenAI format)

Supports `deepseek-chat` and `deepseek-reasoner` models, with optional thinking and web search.

**Non-streaming with thinking + search:**

```bash
curl -X POST http://localhost:3000/v1/chat/completions \
  -H "Authorization: Bearer deepseek-proxy" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "deepseek-reasoner",
    "messages": [{"role": "user", "content": "What is the latest news about AI?"}],
    "thinking": true,
    "search": true,
    "stream": false
  }'
```

**Streaming with thinking + search:**

```bash
curl -X POST http://localhost:3000/v1/chat/completions \
  -H "Authorization: Bearer deepseek-proxy" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "deepseek-reasoner",
    "messages": [{"role": "user", "content": "Explain quantum computing in simple terms"}],
    "thinking": true,
    "search": true,
    "stream": true
  }'
```

### Multi-turn conversation example

Enable history first, then send messages sequentially — the model retains context across requests.

```bash
# Step 1: Enable history
curl -X POST http://localhost:3000/history \
  -H "Authorization: Bearer deepseek-proxy" \
  -H "Content-Type: application/json" \
  -d '{"enable": true}'

# Step 2: First message
curl -X POST http://localhost:3000/v1/chat/completions \
  -H "Authorization: Bearer deepseek-proxy" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "deepseek-chat",
    "messages": [{"role": "user", "content": "My name is John"}],
    "thinking": false,
    "search": false,
    "stream": false
  }'

# Step 3: Follow-up — model should remember the name
curl -X POST http://localhost:3000/v1/chat/completions \
  -H "Authorization: Bearer deepseek-proxy" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "deepseek-chat",
    "messages": [{"role": "user", "content": "What is my name?"}],
    "thinking": false,
    "search": false,
    "stream": false
  }'
```

> The second request should return "John" — history is preserved across calls.

---

## Acknowledgements

- [github.com/xtekky/deepseek4free](https://github.com/xtekky/deepseek4free)
