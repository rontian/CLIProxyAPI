# Auto Model Routing Tasks

## Maintenance and Merge Governance

- [x] Create a local-extension merge guide covering embedding, OAuth model aliases, image/video generation, and Auto Router.
- [x] Record branch-sync rules for keeping `main` aligned with upstream and merging into `main_ai`.
- [x] Add task/status maintenance rules to `AGENTS.md`.
- [x] Use typed Chinese commit messages for future local-extension work.
- [x] Add Python-free config sync path for production hosts.
- [x] Make Docker build mirrors configurable while keeping domestic defaults.

## V1 Stable Router

- [x] Add auto-router config schema.
- [x] Normalize auto-router model, role, session, and fallback config.
- [x] Add built-in auto router package.
- [x] Support deterministic role matching with sticky session binding.
- [x] Preserve thinking suffixes when routing `auto(...)`.
- [x] Route built-in auto decisions before plugin model routers.
- [x] Add route metadata for selected role, reason, and sticky hit.
- [x] Expose configured auto models through OpenAI model listing.
- [x] Add unit tests for config normalization and routing behavior.
- [x] Add model-backed brain judge execution.
- [x] Inject selected role prompt templates into OpenAI Chat and Responses requests.
- [x] Add switch policy for explicit topic changes.
- [x] Add management API endpoints for auto-router config.
- [x] Add sticky session inspection and clearing endpoints.
- [x] Add deterministic dry-run route testing endpoint.
- [x] Add CPA-Manager UI for role editing and dry-run route testing.
- [x] Add built-in and custom role preset management in CPA-Manager.

## V1 Validation

- [x] Run `gofmt` on changed Go files.
- [x] Run focused tests for auto-router, config, handler, and OpenAI handler packages.
- [ ] Run `go test ./...` cleanly. Current unrelated failures are in pluginhost, runtime executor, and Gemini Responses translator tests.
- [x] Run `go build -o test-output ./cmd/server && rm test-output`.

## V2 Runtime Direction

- [x] Add Copilot-style Auto Model Selection v2 design documentation.
- [x] Add auto-router policy and per-role model candidate schema.
- [x] Add request complexity scoring for auto-router candidate selection.
- [x] Select role model candidates by strategy, complexity, cost, and priority.
- [x] Add tests for candidate normalization and policy-based selection.
- [ ] Design `auto-agent` runtime state model.
- [ ] Add planner/worker/reviewer/finalizer step types.
- [ ] Add trace records for each model call.
- [ ] Add cost, step, and timeout limits.
- [ ] Add interruption/resume semantics.
- [ ] Add CPA-Manager runtime trace viewer.

## GitHub Copilot Provider Plugin

- [x] Document the GitHub Copilot provider boundary and plugin-based implementation approach.
- [x] Add a dynamic `github-copilot` provider plugin with device login, token refresh, model metadata, and chat-completions execution.
- [x] Route plugin HTTP traffic through host callbacks so proxy config and request logging still apply.
- [x] Add example plugin config and build/run instructions.
- [x] Keep maintained plugin source under `plugins-src/` and deploy compiled artifacts to runtime `plugins/`.
- [x] Validate plugin build and core server compile.
- [x] Integrate Copilot plugin build into local Makefile development flow.
- [x] Package Copilot plugin artifact in Docker images.
- [x] Update plugin build and deployment documentation.
- [x] Verify CPA-Manager can discover OAuth-capable plugins through the existing management plugin list API.
- [x] Document that plugin providers need a generic management UI entry rather than provider-specific hardcoding.
- [x] Fix Copilot plugin login startup so unreachable GitHub device-code requests fail quickly.
- [x] Keep plugin device-flow UI from showing waiting state before an authorization URL is available.
- [x] Add safe management logs for plugin OAuth polling status without leaking tokens.
- [x] Let management model definitions fall back to runtime provider models for plugin OAuth providers.
- [x] Diagnose GitHub Copilot plugin model routing failures from OpenAI-compatible clients.
- [x] Add Makefile dev preflight that stops stale local CLIProxyAPI listeners before starting the dev server.
- [x] Fix GitHub Copilot streaming chunks so Chat Completions clients receive valid SSE frames.
- [x] Publish usage records for plugin executor requests so CPA-Manager realtime monitoring shows Copilot calls.
- [x] Diagnose why GitHub Copilot Claude-family models are missing from management model lists.
- [x] Fix dev preflight stale Go server detection for macOS `go run` temp executables.
- [x] Fix ZCode streaming calls to GitHub Copilot by sanitizing unsupported OpenAI extension fields before plugin forwarding.
- [x] Make local `make dev` shutdown deterministic when interrupted during plugin unload.
- [x] Fix GitHub Copilot SSE framing so partial upstream read chunks are buffered before emitting JSON frames.
- [x] Diagnose NAS GitHub Copilot OAuth token exchange failures and confirm plugin host calls honor configured SOCKS5 proxies.
- [x] Add safe plugin host HTTP proxy diagnostics for NAS GitHub Copilot OAuth failures.
- [x] Add GitHub Copilot plugin-level proxy override for OAuth and Copilot API requests.
- [ ] Design a safe manual model supplement path for provider/auth model lists without making unsupported models look callable.
- [ ] Investigate a non-default transport for GitHub Copilot Claude models; Go `net/http` currently receives a reduced model set and `model_not_supported` for Claude while VS Code/Python can access some Claude models.
