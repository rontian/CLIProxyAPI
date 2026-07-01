package config

import "testing"

func TestParseConfigBytesSanitizesAutoRouter(t *testing.T) {
	cfg, err := ParseConfigBytes([]byte(`
auto-router:
  enabled: true
  role-presets:
    - id: " Debug "
      name: " Debug Preset "
      cost-tier: " HIGH "
      strengths: [" Logs ", "Logs"]
      match-keywords: [" Error ", "error"]
      prompt-template: " diagnose "
    - id: "debug"
      name: "duplicate"
    - id: ""
      name: "ignored"
  models:
    - name: " auto "
      policy:
        strategy: " COST-FIRST "
      default-role: " coding "
      session:
        enabled: true
        ttl: "bad"
        switch-threshold: 2
        switch-keywords: [" New Task ", "new task"]
        key-sources: [" headers.x-session-id ", "headers.x-session-id"]
      fallback:
        provider: " Claude "
        model: " claude-sonnet "
      roles:
        - id: " coding "
          provider: " Codex "
          model: " gpt-5-codex "
          candidates:
            - provider: " Gemini "
              model: " gemini-2.5-flash "
              cost-tier: " LOW "
              capability-tier: " MEDIUM "
              min-complexity: " LOW "
              max-complexity: " MEDIUM "
            - provider: "gemini"
              model: "gemini-2.5-flash"
            - provider: ""
              model: "ignored"
          match-keywords: [" Docker ", "docker"]
        - id: ""
          provider: "gemini"
          model: "gemini-flash"
`))
	if err != nil {
		t.Fatalf("ParseConfigBytes() error = %v", err)
	}
	if !cfg.AutoRouter.Enabled {
		t.Fatal("AutoRouter.Enabled = false, want true")
	}
	model := cfg.AutoRouter.Models[0]
	if model.Name != "auto" || model.Fallback.Provider != "claude" || model.Fallback.Model != "claude-sonnet" {
		t.Fatalf("model = %+v, want sanitized names", model)
	}
	if model.Policy.Strategy != "cost-first" {
		t.Fatalf("policy = %+v, want cost-first strategy", model.Policy)
	}
	if model.Session.TTL != defaultAutoRouterSessionTTL || model.Session.SwitchThreshold != 1 {
		t.Fatalf("session = %+v, want default ttl and clamped threshold", model.Session)
	}
	if len(model.Session.KeySources) != 1 || model.Session.KeySources[0] != "headers.x-session-id" {
		t.Fatalf("key sources = %v, want one trimmed source", model.Session.KeySources)
	}
	if len(model.Session.SwitchKeywords) != 1 || model.Session.SwitchKeywords[0] != "new task" {
		t.Fatalf("switch keywords = %v, want one normalized keyword", model.Session.SwitchKeywords)
	}
	if len(model.Roles) != 1 || model.Roles[0].Provider != "codex" || model.Roles[0].MatchKeywords[0] != "docker" {
		t.Fatalf("roles = %+v, want sanitized coding role", model.Roles)
	}
	if len(model.Roles[0].Candidates) != 1 {
		t.Fatalf("candidates = %+v, want one unique valid candidate", model.Roles[0].Candidates)
	}
	candidate := model.Roles[0].Candidates[0]
	if candidate.Provider != "gemini" || candidate.Model != "gemini-2.5-flash" || candidate.CostTier != "low" || candidate.CapabilityTier != "medium" || candidate.MinComplexity != "low" || candidate.MaxComplexity != "medium" {
		t.Fatalf("candidate = %+v, want sanitized candidate metadata", candidate)
	}
	if len(cfg.AutoRouter.RolePresets) != 1 {
		t.Fatalf("role presets = %+v, want one unique preset", cfg.AutoRouter.RolePresets)
	}
	preset := cfg.AutoRouter.RolePresets[0]
	if preset.ID != "Debug" || preset.Name != "Debug Preset" || preset.CostTier != "high" {
		t.Fatalf("preset = %+v, want sanitized metadata", preset)
	}
	if len(preset.MatchKeywords) != 1 || preset.MatchKeywords[0] != "error" {
		t.Fatalf("preset match keywords = %v, want normalized unique keyword", preset.MatchKeywords)
	}
	if preset.PromptTemplate != "diagnose" {
		t.Fatalf("preset prompt = %q, want trimmed prompt", preset.PromptTemplate)
	}
}
