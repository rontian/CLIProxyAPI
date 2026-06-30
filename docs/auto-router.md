# Auto Router

## Goal

`auto` is a client-visible model that routes each conversation to the most suitable configured role model while preserving conversation continuity. The first version is a stable model gateway feature, not a full multi-agent runtime.

## Scope

Version 1 provides:

- Configurable auto model names, starting with `auto`.
- Configurable roles, each with a provider, target model, prompt template metadata, strengths, cost tier, and match keywords.
- Optional model-backed brain judge that returns structured JSON.
- Session stickiness so follow-up turns stay with the same role by default.
- Role prompt injection for OpenAI Chat and Responses requests.
- Conservative fallback to a configured provider/model when no role matches.
- Route metadata for logs and future management UI: selected role, reason, and sticky hit.
- `/v1/models` visibility for configured auto models.

Version 1 intentionally does not:

- Run several expert models for one response.
- Merge answers from multiple models.
- Rewrite or summarize user history before forwarding to the selected expert model.
- Execute tools or shell commands as a runtime.

## Architecture

```text
client request model=auto
        |
        v
built-in auto router
        |
        |-- sticky session hit -> previous role/provider/model
        |
        |-- no sticky hit -> role selection / fallback
        v
existing provider and auth manager path
        |
        v
selected upstream model response
```

The selected expert receives the original conversation history. For OpenAI Chat requests, the role prompt is prepended as a system message. For OpenAI Responses requests, the role prompt is prepended to `instructions`. This keeps user history intact while giving the selected expert a role-specific operating frame.

## Configuration

```yaml
auto-router:
  enabled: true
  models:
    - name: "auto"
      description: "Stable role-based model router"
      default-role: "fast"
      fallback:
        provider: "claude"
        model: "claude-sonnet-4-5"
      brain:
        provider: "gemini"
        model: "gemini-2.5-flash"
        temperature: 0
        max-tokens: 512
      session:
        enabled: true
        ttl: "30m"
        switch-threshold: 0.85
        max-switches: 3
        switch-keywords:
          - "new task"
          - "new topic"
          - "switch to"
        key-sources:
          - "metadata.execution_session_id"
          - "headers.x-session-id"
          - "body.metadata.user_id"
          - "history-hash"
      roles:
        - id: "coding"
          name: "Coding Agent"
          provider: "codex"
          model: "gpt-5-codex"
          cost-tier: "high"
          priority: 100
          strengths:
            - "code editing"
            - "debugging"
            - "repo analysis"
          match-keywords:
            - "stack trace"
            - "docker"
            - "go test"
          prompt-template: |
            You are a senior coding assistant. Focus on implementation correctness.

        - id: "fast"
          name: "Fast Agent"
          provider: "gemini"
          model: "gemini-2.5-flash"
          cost-tier: "low"
          priority: 10
          strengths:
            - "short Q&A"
            - "translation"
            - "summary"
```

When `brain.provider` and `brain.model` are configured, the router asks the judge model for JSON before deterministic matching. The judge may only select configured roles (or `fallback`); the concrete provider/model still comes from configuration.

Expected judge output:

```json
{
  "role_id": "coding",
  "confidence": 0.91,
  "reason": "The request is asking to debug and modify code."
}
```

If the judge call fails, returns invalid JSON, or selects an unknown role, the router falls back to deterministic matching, default role, then fallback target.

## Session Continuity

The router derives a session key from configured sources first, then common defaults:

- `metadata.execution_session_id`
- `X-Session-ID`, `Session-Id`, `Session_id`, `X-Client-Request-Id`
- `session_id`, `conversation_id`, `metadata.session_id`, `metadata.conversation_id`, `metadata.user_id`
- a hash of the first few messages when `fallback-history-hash` is enabled

Once a role is bound, follow-up turns use the same provider/model until the TTL expires. The selected expert still receives the full original request history.

Sticky sessions are intentionally conservative. The router only re-evaluates a bound session when the request has an explicit switch signal:

- `X-Auto-Route-Reset: true` or `X-Auto-Router-Reset: true`
- `auto_route_reset=true` or `auto_router_reset=true` query parameters
- `metadata.auto_route_reset=true` or `metadata.auto_router_reset=true` in the request body
- configured `session.switch-keywords`, or the built-in English defaults such as `new task`, `new topic`, and `switch to`

When a switch is evaluated, the new candidate must meet `session.switch-threshold`; deterministic keyword matches count as high confidence. `session.max-switches` limits how many times one sticky binding can change roles. A value of `0` means no explicit limit.

## Management API

The first backend management surface exposes the config block:

- `GET /v0/management/auto-router`
- `PUT /v0/management/auto-router`
- `PATCH /v0/management/auto-router`
- `DELETE /v0/management/auto-router`
- `GET /v0/management/auto-router/sessions`
- `DELETE /v0/management/auto-router/sessions`
- `POST /v0/management/auto-router/dry-run`

`PUT` and `PATCH` accept either the raw auto-router object or a wrapped object:

```json
{
  "auto-router": {
    "enabled": true,
    "models": []
  }
}
```

The dry-run endpoint previews deterministic routing without calling the brain model and without creating sticky sessions:

```json
{
  "model": "auto",
  "source_format": "openai",
  "headers": {
    "X-Session-Id": ["preview"]
  },
  "body": {
    "messages": [
      {"role": "user", "content": "docker build failed"}
    ]
  }
}
```

## Later Runtime Direction

The second major phase should add a separate runtime, not overload the router:

```text
auto-agent runtime
  planner -> worker role(s) -> reviewer -> finalizer
```

That runtime needs task state, traces, tool permissions, interruption/resume behavior, and cost controls. The router remains the entry layer that decides whether a request is simple enough for single-expert routing or complex enough for runtime orchestration.
