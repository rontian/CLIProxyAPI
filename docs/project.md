# Project: Auto Model Routing

## Objective

Add a configurable `auto` model to CLIProxyAPI that routes user requests to role-specific expert models while keeping conversation context stable and avoiding unnecessary model cost.

## Product Shape

The first release is a stable model router:

- The user selects `model=auto`.
- CLIProxyAPI chooses a configured role agent.
- The selected role maps to one concrete provider/model.
- The original conversation history is forwarded to that selected expert.
- The selected role prompt is injected as system-level guidance where the request protocol supports it.
- Session stickiness keeps follow-up turns on the same role unless the binding expires.
- Explicit switch signals can re-evaluate a sticky session, but routine follow-up turns do not re-route.

The feature is deliberately not a full multi-agent runtime in the first release.

## Key Requirements

- Roles must be configurable with provider, model, prompt template metadata, strengths, cost tier, priority, and matching hints.
- The router must preserve history correctness by forwarding the original request body to the selected model.
- The router may add role guidance, but must not drop, reorder, or summarize user history.
- Sticky sessions must prevent accidental role switching in follow-up turns.
- Explicit switch handling must be bounded by confidence and max-switch limits.
- The router must expose enough metadata for request logs and CPA-Manager.
- The configuration must be suitable for CPA-Manager UI editing.
- The implementation must stay before provider/auth selection and avoid translator-only changes.

## Non-Goals For V1

- Multi-model collaboration in a single user turn.
- Automatic answer synthesis from several models.
- Tool execution orchestration.
- Hidden runtime memory beyond route bindings.
- Provider-specific request translation changes.

## V1 Architecture

```text
OpenAI/Responses request
  -> BaseAPIHandler.applyModelRouter
  -> built-in auto router
  -> provider/model decision
  -> existing AuthManager execution
```

The built-in router runs before plugin model routers. It only handles configured auto model names and leaves every other model untouched.

## V2 Direction

V2 can add an `auto-agent` runtime:

- planner model for task decomposition
- one or more worker roles
- reviewer/checker role
- finalizer role
- runtime traces
- cost and step limits
- CPA-Manager visualization

This should live beside the router as a separate runtime module. The router can decide when to hand off to the runtime.
