# Go-side /ai Reverse Proxy Design

Date: 2026-11-11
Status: Approved (user confirmed)

## Goal

Make AI editing work in production: proxy `/ai/*` from the Go server to the
AI gateway, replicating vite dev's proxy semantics.

## Semantics (mirrors vite.config.js aiProxy)

- Route `/ai/*` → `AI_PROXY_TARGET` with the `/ai` prefix stripped
  (`/ai/v1/chat/completions` → `<target>/v1/chat/completions`).
- Inject `Authorization: Bearer $AI_PROXY_KEY` server-side, REPLACING any
  client-sent Authorization header (the key never ships to the browser).
- Config source: the repo's `.env` + `.env.local` (same files vite reads);
  the systemd unit gains `EnvironmentFile=` lines for both. When
  `AI_PROXY_TARGET` is unset, `/ai` stays unrouted (404, current behavior).
- Session gate: `/ai` sits behind `requireAuth` — the gateway is only
  reachable through this proxy (127.0.0.1), and an ungated proxy would let
  anonymous LAN users burn the gateway quota.
- Streaming: stdlib `httputil.ReverseProxy` passes SSE through natively.

## Files

- Create: `server-go/ai.go` (`aiProxyHandler(target, key string) http.Handler`)
- Test: `server-go/ai_test.go` (httptest fake gateway: prefix stripped,
  Authorization injected/overridden, 401 without cookie, body/status
  passthrough)
- Modify: `server-go/main.go` (mount `/ai/` behind requireAuth when
  configured), `systemd/mermaid-server.service` (EnvironmentFile .env +
  .env.local), `README-LOCAL.md` (remove the "AI 生产不可用" caveats; add
  the config note)
- Cutover: `bash install.sh`; verify `/ai/v1/models` → 401 without a
  session (routing + gate live).

## Out of Scope

- Rate limiting on the proxy; gateway-side concerns.
