# DeepSeek Python Api (Free)

A Python DeepSeek API proxy with Cloudflare bypass capabilities and OpenAI-compatible Api format.

## Features

- Full DeepSeek API implementation
- Cloudflare protection bypass
- Proof of Work (PoW) challenge solving
- OpenAI-compatible proxy server
- Interactive CLI chat client
- Cookie management system
- Streaming and non-streaming responses
- Threaded conversation support

## Installation

```bash
# Clone the repository
git clone https://github.com/izaart95-jpg/DeepRouter.git
cd DeepRouter
```

# Install dependencies
```bash
pip install -r requirements.txt
```
# Get Deepseek Token
Go to chat.deepseek.com login if required
open devtools and run
```bash
JSON.parse(localStorage.getItem("userToken")).value
```

# Copy environment variables
```
cp .env.example .env
```
# Edit .env with your DeepSeek token 
```bash
# OR
export DEEPSEEK_TOKEN=token  # without quotes
```

# Run Interactive Chat
```bash
python interactive_chat.py
```

# Run Openai compaitable api proxy
```bash
python proxy.py
```

## Turn History On
```bash
curl -X POST http://localhost:3000/history \
  -H "Authorization: Bearer deepseek-proxy" \
  -H "Content-Type: application/json" \
  -d '{"enable": true}'
```
## Turn History Off
```bash
curl -X POST http://localhost:3000/history \
  -H "Authorization: Bearer deepseek-proxy" \
  -H "Content-Type: application/json" \
  -d '{"enable": false}'
```
## Create new session
```bash
curl -X POST http://localhost:3000/new \
  -H "Authorization: Bearer deepseek-proxy"
```
## Search and Thinking enabled - No Streaming
```bash
curl -X POST http://localhost:3000/v1/chat/completions \
  -H "Authorization: Bearer deepseek-proxy" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "deepseek-reasoner",
    "messages": [
      {"role": "user", "content": "What is the latest news about AI?"}
    ],
    "thinking": true,
    "search": true,
    "stream": false
  }'
```
## Search and Thinking With Stream
```bash
curl -X POST http://localhost:3000/v1/chat/completions \
  -H "Authorization: Bearer deepseek-proxy" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "deepseek-reasoner",
    "messages": [
      {"role": "user", "content": "Explain quantum computing in simple terms"}
    ],
    "thinking": true,
    "search": true,
    "stream": true
  }'
```
## Conversation Example - History On
``` # First, enable history
curl -X POST http://localhost:3000/history \
  -H "Authorization: Bearer deepseek-proxy" \
  -H "Content-Type: application/json" \
  -d '{"enable": true}'

# First message
curl -X POST http://localhost:3000/v1/chat/completions \
  -H "Authorization: Bearer deepseek-proxy" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "deepseek-chat",
    "messages": [
      {"role": "user", "content": "My name is John"}
    ],
    "thinking": false,
    "search": false,
    "stream": false
  }'

# Second message - should remember your name
curl -X POST http://localhost:3000/v1/chat/completions \
  -H "Authorization: Bearer deepseek-proxy" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "deepseek-chat",
    "messages": [
      {"role": "user", "content": "What is my name?"}
    ],
    "thinking": false,
    "search": false,
    "stream": false
  }'
  ```

### Acknowledgements
https://github.com/xtekky/deepseek4free
