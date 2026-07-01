# Auto Model Selection V2

## Goal

Auto Router V2 moves the `auto` model toward a Copilot-style automatic model
selection layer while preserving the existing role-router guarantees.

The router should still choose one concrete provider/model for one user turn.
It should not become a multi-step agent runtime in this phase.

## Product Direction

The target behavior is:

- keep `model=auto` as the client-visible entry point;
- use the existing brain judge and deterministic rules to select the role;
- let each role define a candidate model pool instead of one fixed model;
- classify request complexity from the current request and recent history;
- score candidates by policy, complexity fit, cost tier, capability tier, and
  priority;
- keep sticky sessions at conversation boundaries so follow-up turns do not
  churn between models;
- expose route metadata so the manager can explain why a model was selected.

This is compatible with the V1 role router. Old configs that only use
`roles[].provider` and `roles[].model` continue to behave the same.

## Difference From Full Agent Runtime

This phase is model selection, not orchestration.

It does not:

- call several models for one answer;
- split a task into planner/worker/reviewer steps;
- synthesize multiple model outputs;
- add tool execution or durable task state.

Those belong to a later `auto-agent` runtime.

## Configuration Shape

```yaml
auto-router:
  enabled: true
  models:
    - name: "auto"
      policy:
        strategy: "balanced" # balanced | cost-first | quality-first
      session:
        enabled: true
        ttl: "30m"
      fallback:
        provider: "gemini"
        model: "gemini-2.5-flash"
      roles:
        - id: "coding"
          provider: "codex"
          model: "gpt-5-codex"
          candidates:
            - provider: "openai-compatible-deepseek"
              model: "deepseek-v4-flash"
              cost-tier: "low"
              capability-tier: "medium"
              priority: 80
              min-complexity: "low"
              max-complexity: "medium"
            - provider: "codex"
              model: "gpt-5-codex"
              cost-tier: "high"
              capability-tier: "high"
              priority: 100
              min-complexity: "high"
```

The legacy role target remains the fallback candidate for that role. This keeps
existing configs valid and avoids forcing users to fill candidate pools.

## Selection Flow

```text
request model=auto
  -> sticky session hit?
       yes -> previous role/provider/model
       no  -> judge/rule role selection
  -> role candidate pool
  -> request complexity classifier
  -> policy scoring
  -> selected provider/model
  -> sticky session bind
  -> existing provider/auth execution path
```

## Strategy Semantics

- `balanced`: prefer candidates that fit complexity, then cost and capability.
- `cost-first`: strongly prefer lower cost when the request complexity allows it.
- `quality-first`: strongly prefer higher capability for medium/high complexity.

The first implementation uses deterministic heuristic complexity scoring.
Later versions can add model-health signals from request logs:

- recent success/failure rate;
- cooling/quota state;
- median latency;
- recent 429/503 failures;
- provider disabled state.

## Session Boundary

Sticky sessions stay central. A selected candidate is bound with the selected
role. Routine follow-up turns reuse the same binding. Explicit switch signals
can re-evaluate the role and candidate, bounded by `switch-threshold` and
`max-switches`.

This mirrors the practical behavior of hosted Auto modes: avoid changing models
inside the same conversation unless there is a clear reason.

## Rollout Plan

1. Add policy and candidate schema with backward-compatible normalization.
2. Add deterministic complexity scoring and candidate scoring.
3. Surface selected complexity/strategy in decision metadata.
4. Add CPA-Manager controls for policy and candidates.
5. Add runtime health signals from recent request metrics.
6. Add a route trace view in CPA-Manager.
