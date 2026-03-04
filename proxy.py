#!/usr/bin/env python3
"""
DeepSeek OpenAI-Compatible Proxy  (v6 — source verified from working api.py)
=============================================================================
Chunk types yielded by the patched api.py chat_completion():
  {'type': 'ready',          'request_message_id': N, 'response_message_id': N}
  {'type': 'session_update', 'updated_at': float}
  {'type': 'content',        'content': str, 'full_content': str}   ← TEXT
  {'type': 'status',         'status': 'FINISHED'}
  {'type': 'complete',       'finish_reason': 'stop'}
  {'type': 'title',          'content': str}

Setup:
  export DEEPSEEK_TOKEN="eyJ..."
  export PROXY_API_KEY="deepseek-proxy"
  python proxy.py
"""

import os, sys, json, time, uuid, threading, logging
from http.server import HTTPServer, BaseHTTPRequestHandler
from socketserver import ThreadingMixIn
from urllib.parse import urlparse, parse_qs

# ── import api.py (same directory) ───────────────────────────────────────────
try:
    from api import (
        DeepSeekAPI,
        AuthenticationError, RateLimitError,
        NetworkError, CloudflareError, APIError,
    )
except ImportError:
    print("[ERROR] Cannot import api.py — place proxy.py in the same folder.", file=sys.stderr)
    sys.exit(1)

# ── config ────────────────────────────────────────────────────────────────────
PORT           = int(os.getenv("PORT",          3000))
PROXY_API_KEY  = os.getenv("PROXY_API_KEY",  "deepseek-proxy")
DEEPSEEK_TOKEN = os.getenv("DEEPSEEK_TOKEN", "")

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s  %(levelname)-8s  %(message)s",
    datefmt="%H:%M:%S",
)
log = logging.getLogger("ds-proxy")

MODELS = [
    {"id": "deepseek-chat",     "object": "model", "created": 1700000000, "owned_by": "deepseek"},
    {"id": "deepseek-reasoner", "object": "model", "created": 1700000000, "owned_by": "deepseek"},
]

# ── singleton API ─────────────────────────────────────────────────────────────
_api_lock = threading.Lock()
_api = None

def get_api():
    global _api
    if _api: return _api
    with _api_lock:
        if not _api:
            if not DEEPSEEK_TOKEN:
                raise RuntimeError(
                    "DEEPSEEK_TOKEN not set.\n"
                    "  export DEEPSEEK_TOKEN=<chat.deepseek.com localStorage -> userToken -> value>"
                )
            _api = DeepSeekAPI(DEEPSEEK_TOKEN)
            log.info("DeepSeekAPI ready")
    return _api

# ── history state ─────────────────────────────────────────────────────────────
_hl           = threading.Lock()
_hist_chat_id = None
_hist_par_id  = None   # response_message_id from last DS response
_use_history  = False

def _new_hist():
    global _hist_chat_id, _hist_par_id
    cid = get_api().create_chat_session()
    with _hl: _hist_chat_id, _hist_par_id = cid, None
    log.info("New history session: %s", cid)
    return cid

def _get_hist():
    with _hl: return _hist_chat_id, _hist_par_id

def _set_hist_par(mid):
    global _hist_par_id
    with _hl: _hist_par_id = mid

# ── threaded server ───────────────────────────────────────────────────────────
class ThreadingHTTPServer(ThreadingMixIn, HTTPServer):
    daemon_threads = True

# ── handler ───────────────────────────────────────────────────────────────────
class Handler(BaseHTTPRequestHandler):
    def log_message(self, fmt, *args): pass

    def _body(self):
        n = int(self.headers.get("Content-Length", 0))
        return json.loads(self.rfile.read(n)) if n else {}

    def _json(self, data, status=200):
        b = json.dumps(data).encode()
        self.send_response(status)
        self.send_header("Content-Type",   "application/json")
        self.send_header("Content-Length", str(len(b)))
        self._cors(); self.end_headers(); self.wfile.write(b)

    def _err(self, msg, etype="server_error", code=None, status=500):
        self._json({"error": {"message": msg, "type": etype,
                               "param": None, "code": code}}, status)

    def _cors(self):
        self.send_header("Access-Control-Allow-Origin",  "*")
        self.send_header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
        self.send_header("Access-Control-Allow-Headers", "Content-Type, Authorization")

    def _ok(self):
        if not PROXY_API_KEY: return True
        return self.headers.get("Authorization", "") == f"Bearer {PROXY_API_KEY}"

    # verbs ────────────────────────────────────────────────────────────────────

    def do_OPTIONS(self):
        self.send_response(200); self._cors(); self.end_headers()

    def do_GET(self):
        if not self._ok():
            return self._err("Bad token", "authentication_error", "invalid_api_key", 401)
        parsed = urlparse(self.path)
        qs, path = parse_qs(parsed.query), parsed.path

        if path in ("/v1/models", "/models"):
            self._json({"object": "list", "data": MODELS})

        elif path == "/history":
            global _use_history
            v = qs.get("enable", qs.get("value", ["false"]))[0]
            _use_history = v.lower() == "true"
            cid, _ = _get_hist()
            self._json({"use_history": _use_history, "chat_id": cid})

        else:
            self._err("Not Found", "invalid_request_error", "not_found", 404)

    def do_POST(self):
        if not self._ok():
            return self._err("Bad token", "authentication_error", "invalid_api_key", 401)
        path = urlparse(self.path).path
        try:
            if path == "/new":
                try: get_api()
                except RuntimeError as e:
                    return self._err(str(e), "authentication_error", "missing_token", 401)
                self._json({"message": "New session", "chat_id": _new_hist()})

            elif path == "/history":
                body = self._body()
                global _use_history
                _use_history = bool(body.get("enable", body.get("value", False)))
                self._json({"use_history": _use_history})

            elif path in ("/v1/chat/completions", "/chat/completions"):
                self._chat()

            else:
                self._err("Not Found", "invalid_request_error", "not_found", 404)

        except Exception as exc:
            log.exception("Unhandled: %s", exc)
            try: self._err(str(exc))
            except Exception: pass

    # ── /v1/chat/completions ──────────────────────────────────────────────────

    def _chat(self):
        body     = self._body()
        messages = body.get("messages", [])
        model    = body.get("model",    "deepseek-chat")
        stream   = body.get("stream",   True)
        thinking = (
            body.get("thinking",  False) or
            body.get("deepThink", False) or
            model == "deepseek-reasoner"
        )
        search = body.get("search", False)

        # prompt = content of last message
        raw = messages[-1].get("content", "") if messages else ""
        if isinstance(raw, list):
            prompt = "\n".join(p["text"] for p in raw if p.get("type") == "text")
        else:
            prompt = raw or " "

        try:
            api = get_api()
        except RuntimeError as e:
            return self._err(str(e), "authentication_error", "missing_token", 401)

        # session
        if _use_history:
            chat_id, par_id = _get_hist()
            if not chat_id:
                try:   chat_id = _new_hist(); par_id = None
                except Exception as e:
                    return self._err(str(e), "upstream_error", None, 502)
        else:
            # stateless: fresh session every request
            try:
                chat_id = api.create_chat_session()
                par_id  = None
                log.info("Stateless session: %s", chat_id)
            except Exception as e:
                return self._err(str(e), "upstream_error", None, 502)

        log.info("→ model=%-20s think=%-5s search=%-5s history=%s",
                 model, thinking, search, _use_history)

        if stream:
            self._stream(api, chat_id, par_id, prompt, model, thinking, search)
        else:
            self._block(api, chat_id, par_id, prompt, model, thinking, search)

    # ── streaming ─────────────────────────────────────────────────────────────

    def _stream(self, api, chat_id, par_id, prompt, model, thinking, search):
        self.close_connection = True  # drop TCP after stream ends
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.send_header("Cache-Control","no-cache")
        self.send_header("Connection",   "close")
        self._cors(); self.end_headers()

        rid     = "chatcmpl-" + uuid.uuid4().hex
        created = int(time.time())

        def sse(obj):
            try:
                self.wfile.write(f"data: {json.dumps(obj)}\n\n".encode())
                self.wfile.flush()
            except (BrokenPipeError, ConnectionResetError): pass

        # role-establishing chunk (required by most clients)
        sse({"id": rid, "object": "chat.completion.chunk", "created": created,
             "model": model, "choices": [{"index": 0,
             "delta": {"role": "assistant", "content": ""}, "finish_reason": None}]})

        params = {
            "chat_session_id":  chat_id,
            "prompt":           prompt,
            "thinking_enabled": thinking,
            "search_enabled":   search,
        }
        if par_id is not None:
            params["parent_message_id"] = par_id

        try:
            for chunk in api.chat_completion(**params):
                ctype = chunk.get("type", "")

                # ── capture response_message_id for history threading ─────────
                if ctype == "ready" and _use_history:
                    mid = chunk.get("response_message_id")
                    if mid: _set_hist_par(mid)

                # ── only 'content' chunks carry text to forward ───────────────
                if ctype != "content":
                    continue

                text = chunk.get("content", "")
                if not text:
                    continue

                sse({"id": rid, "object": "chat.completion.chunk",
                     "created": created, "model": model,
                     "choices": [{"index": 0,
                                  "delta": {"content": text},
                                  "finish_reason": None}]})

        except AuthenticationError as e:
            sse({"error": {"message": str(e), "type": "authentication_error"}}); return
        except RateLimitError as e:
            sse({"error": {"message": str(e), "type": "rate_limit_error"}}); return
        except CloudflareError as e:
            sse({"error": {"message": str(e), "type": "cloudflare_error"}}); return
        except (NetworkError, APIError) as e:
            sse({"error": {"message": str(e), "type": "api_error"}}); return
        except (BrokenPipeError, ConnectionResetError): return
        except Exception as e:
            log.exception("Stream error: %s", e); return

        # stop chunk
        sse({"id": rid, "object": "chat.completion.chunk", "created": created,
             "model": model, "choices": [{"index": 0, "delta": {},
             "finish_reason": "stop"}]})
        try:
            self.wfile.write(b"data: [DONE]\n\n"); self.wfile.flush()
        except (BrokenPipeError, ConnectionResetError, OSError): pass
        finally:
            try: self.wfile.close()
            except OSError: pass
        self.close_connection = True

    # ── non-streaming ─────────────────────────────────────────────────────────

    def _block(self, api, chat_id, par_id, prompt, model, thinking, search):
        parts = []

        params = {
            "chat_session_id":  chat_id,
            "prompt":           prompt,
            "thinking_enabled": thinking,
            "search_enabled":   search,
        }
        if par_id is not None:
            params["parent_message_id"] = par_id

        try:
            for chunk in api.chat_completion(**params):
                ctype = chunk.get("type", "")

                if ctype == "ready" and _use_history:
                    mid = chunk.get("response_message_id")
                    if mid: _set_hist_par(mid)

                if ctype == "content":
                    text = chunk.get("content", "")
                    if text: parts.append(text)

        except AuthenticationError as e: return self._err(str(e),"authentication_error",None,401)
        except RateLimitError      as e: return self._err(str(e),"rate_limit_error",None,429)
        except CloudflareError     as e: return self._err(str(e),"cloudflare_error",None,503)
        except (NetworkError,APIError) as e: return self._err(str(e),"api_error",None,502)

        answer = "".join(parts)
        pt = len(prompt.split()); ct = len(answer.split())
        self._json({
            "id":      "chatcmpl-" + uuid.uuid4().hex,
            "object":  "chat.completion",
            "created": int(time.time()),
            "model":   model,
            "choices": [{"index": 0,
                         "message": {"role": "assistant", "content": answer},
                         "finish_reason": "stop"}],
            "usage":   {"prompt_tokens": pt, "completion_tokens": ct,
                        "total_tokens": pt + ct},
        })

# ── main ──────────────────────────────────────────────────────────────────────

def main():
    log.info("DeepSeek OpenAI Proxy  v6")
    log.info("  Port      : %d", PORT)
    log.info("  Proxy key : %s", PROXY_API_KEY or "(open)")
    log.info("  DS token  : %s", "SET" if DEEPSEEK_TOKEN else "NOT SET  ← export DEEPSEEK_TOKEN=...")
    log.info("  Endpoints : POST /v1/chat/completions")
    log.info("              GET  /v1/models")
    log.info("              GET  /history?enable=true|false")
    log.info("              POST /new")
    server = ThreadingHTTPServer(("0.0.0.0", PORT), Handler)
    log.info("Ready → http://0.0.0.0:%d/v1", PORT)
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        log.info("Bye"); server.server_close()

if __name__ == "__main__":
    main()
