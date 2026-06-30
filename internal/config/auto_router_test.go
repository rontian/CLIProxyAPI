package config

import "testing"

func TestParseConfigBytesSanitizesAutoRouter(t *testing.T) {
	cfg, err := ParseConfigBytes([]byte(`
auto-router:
  enabled: true
  models:
    - name: " auto "
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
}
