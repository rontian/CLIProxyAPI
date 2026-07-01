package autorouter

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
)

func TestRouteMatchesRoleAndKeepsStickySession(t *testing.T) {
	cfg := &config.SDKConfig{AutoRouter: config.AutoRouterConfig{
		Enabled: true,
		Models: []config.AutoModelConfig{{
			Name:        "auto",
			DefaultRole: "fast",
			Session:     config.AutoRouterSessionConfig{Enabled: true, TTL: "30m"},
			Fallback:    config.AutoRouteTargetConfig{Provider: "claude", Model: "claude-sonnet"},
			Roles: []config.AutoRouterRoleConfig{
				{ID: "coding", Provider: "codex", Model: "gpt-5-codex", Priority: 100, MatchKeywords: []string{"docker"}, PromptTemplate: "You are a coding expert."},
				{ID: "fast", Provider: "gemini", Model: "gemini-flash"},
			},
		}},
	}}
	r := New(cfg)
	now := time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)
	r.now = func() time.Time { return now }

	first, ok := r.Route(Request{
		RequestedModel: "auto",
		Headers:        http.Header{"X-Session-Id": []string{"s1"}},
		Body:           []byte(`{"messages":[{"role":"user","content":"fix this docker error"}]}`),
	})
	if !ok {
		t.Fatal("Route() handled = false, want true")
	}
	if first.Provider != "codex" || first.Model != "gpt-5-codex" || first.RoleID != "coding" || first.PromptTemplate != "You are a coding expert." || first.Sticky {
		t.Fatalf("first decision = %+v, want coding codex/gpt-5-codex without sticky", first)
	}

	second, ok := r.Route(Request{
		RequestedModel: "auto",
		Headers:        http.Header{"X-Session-Id": []string{"s1"}},
		Body:           []byte(`{"messages":[{"role":"user","content":"please translate this"}]}`),
	})
	if !ok {
		t.Fatal("Route() second handled = false, want true")
	}
	if second.Provider != "codex" || second.Model != "gpt-5-codex" || second.RoleID != "coding" || second.PromptTemplate != "You are a coding expert." || !second.Sticky {
		t.Fatalf("second decision = %+v, want sticky coding route", second)
	}
	sessions := r.Sessions()
	if len(sessions) != 1 || sessions[0].RoleID != "coding" {
		t.Fatalf("sessions = %+v, want one coding session", sessions)
	}
	if cleared := r.ClearSessions(); cleared != 1 {
		t.Fatalf("ClearSessions() = %d, want 1", cleared)
	}
	if sessions := r.Sessions(); len(sessions) != 0 {
		t.Fatalf("sessions after clear = %+v, want empty", sessions)
	}
}

func TestRouteUsesDefaultRoleAndPreservesSuffix(t *testing.T) {
	cfg := &config.SDKConfig{AutoRouter: config.AutoRouterConfig{
		Enabled: true,
		Models: []config.AutoModelConfig{{
			Name:        "auto",
			DefaultRole: "fast",
			Session:     config.AutoRouterSessionConfig{Enabled: false},
			Fallback:    config.AutoRouteTargetConfig{Provider: "claude", Model: "claude-sonnet"},
			Roles:       []config.AutoRouterRoleConfig{{ID: "fast", Provider: "gemini", Model: "gemini-flash"}},
		}},
	}}
	decision, ok := New(cfg).Route(Request{
		RequestedModel: "auto(high)",
		Body:           []byte(`{"messages":[{"role":"user","content":"hello"}]}`),
	})
	if !ok {
		t.Fatal("Route() handled = false, want true")
	}
	if decision.Provider != "gemini" || decision.Model != "gemini-flash(high)" || decision.RoleID != "fast" {
		t.Fatalf("decision = %+v, want fast route with suffix", decision)
	}
}

func TestRouteUsesFallbackWhenNoRoleMatches(t *testing.T) {
	cfg := &config.SDKConfig{AutoRouter: config.AutoRouterConfig{
		Enabled: true,
		Models: []config.AutoModelConfig{{
			Name:     "auto",
			Session:  config.AutoRouterSessionConfig{Enabled: false},
			Fallback: config.AutoRouteTargetConfig{Provider: "claude", Model: "claude-sonnet"},
			Roles:    []config.AutoRouterRoleConfig{{ID: "coding", Provider: "codex", Model: "gpt-5-codex", MatchKeywords: []string{"docker"}}},
		}},
	}}
	decision, ok := New(cfg).Route(Request{
		RequestedModel: "auto",
		Body:           []byte(`{"messages":[{"role":"user","content":"hello"}]}`),
	})
	if !ok {
		t.Fatal("Route() handled = false, want true")
	}
	if decision.Provider != "claude" || decision.Model != "claude-sonnet" || decision.RoleID != "fallback" {
		t.Fatalf("decision = %+v, want fallback route", decision)
	}
}

func TestRouteWithJudgeUsesConfiguredRole(t *testing.T) {
	cfg := &config.SDKConfig{AutoRouter: config.AutoRouterConfig{
		Enabled: true,
		Models: []config.AutoModelConfig{{
			Name:     "auto",
			Session:  config.AutoRouterSessionConfig{Enabled: false},
			Fallback: config.AutoRouteTargetConfig{Provider: "claude", Model: "claude-sonnet"},
			Brain:    config.AutoRouterBrainConfig{Provider: "gemini", Model: "gemini-flash"},
			Roles: []config.AutoRouterRoleConfig{
				{ID: "coding", Provider: "codex", Model: "gpt-5-codex", PromptTemplate: "Code only."},
				{ID: "fast", Provider: "gemini", Model: "gemini-flash"},
			},
		}},
	}}
	decision, ok := New(cfg).RouteWithJudge(context.Background(), Request{
		RequestedModel: "auto",
		Body:           []byte(`{"messages":[{"role":"user","content":"hello"}]}`),
	}, func(context.Context, BrainRequest) (BrainDecision, error) {
		return BrainDecision{RoleID: "coding", Confidence: 0.9, Reason: "code task"}, nil
	})
	if !ok {
		t.Fatal("RouteWithJudge() handled = false, want true")
	}
	if decision.Provider != "codex" || decision.Model != "gpt-5-codex" || decision.RoleID != "coding" || decision.PromptTemplate != "Code only." || !decision.Brain {
		t.Fatalf("decision = %+v, want brain coding route", decision)
	}
}

func TestRouteWithJudgeFallsBackToDeterministicOnJudgeError(t *testing.T) {
	cfg := &config.SDKConfig{AutoRouter: config.AutoRouterConfig{
		Enabled: true,
		Models: []config.AutoModelConfig{{
			Name:        "auto",
			DefaultRole: "fast",
			Session:     config.AutoRouterSessionConfig{Enabled: false},
			Fallback:    config.AutoRouteTargetConfig{Provider: "claude", Model: "claude-sonnet"},
			Brain:       config.AutoRouterBrainConfig{Provider: "gemini", Model: "gemini-flash"},
			Roles:       []config.AutoRouterRoleConfig{{ID: "fast", Provider: "gemini", Model: "gemini-flash"}},
		}},
	}}
	decision, ok := New(cfg).RouteWithJudge(context.Background(), Request{
		RequestedModel: "auto",
		Body:           []byte(`{"messages":[{"role":"user","content":"hello"}]}`),
	}, func(context.Context, BrainRequest) (BrainDecision, error) {
		return BrainDecision{}, errors.New("judge unavailable")
	})
	if !ok {
		t.Fatal("RouteWithJudge() handled = false, want true")
	}
	if decision.Provider != "gemini" || decision.Model != "gemini-flash" || decision.RoleID != "fast" || decision.Brain {
		t.Fatalf("decision = %+v, want deterministic fast route", decision)
	}
}

func TestPreviewDoesNotCreateStickySession(t *testing.T) {
	cfg := &config.SDKConfig{AutoRouter: config.AutoRouterConfig{
		Enabled: true,
		Models: []config.AutoModelConfig{{
			Name:     "auto",
			Session:  config.AutoRouterSessionConfig{Enabled: true, TTL: "30m"},
			Fallback: config.AutoRouteTargetConfig{Provider: "claude", Model: "claude-sonnet"},
			Roles:    []config.AutoRouterRoleConfig{{ID: "coding", Provider: "codex", Model: "gpt-5-codex", MatchKeywords: []string{"docker"}}},
		}},
	}}
	r := New(cfg)
	decision, ok := r.Preview(Request{
		RequestedModel: "auto",
		Headers:        http.Header{"X-Session-Id": []string{"preview"}},
		Body:           []byte(`{"messages":[{"role":"user","content":"docker build fails"}]}`),
	})
	if !ok || decision.RoleID != "coding" {
		t.Fatalf("Preview() = %+v, %v; want coding decision", decision, ok)
	}
	if sessions := r.Sessions(); len(sessions) != 0 {
		t.Fatalf("Preview() sessions = %+v, want no mutation", sessions)
	}
}

func TestExplicitSwitchCanChangeStickyRole(t *testing.T) {
	cfg := &config.SDKConfig{AutoRouter: config.AutoRouterConfig{
		Enabled: true,
		Models: []config.AutoModelConfig{{
			Name: "auto",
			Session: config.AutoRouterSessionConfig{
				Enabled:         true,
				TTL:             "30m",
				SwitchThreshold: 0.85,
				MaxSwitches:     2,
			},
			Fallback: config.AutoRouteTargetConfig{Provider: "claude", Model: "claude-sonnet"},
			Roles: []config.AutoRouterRoleConfig{
				{ID: "coding", Provider: "codex", Model: "gpt-5-codex", Priority: 100, MatchKeywords: []string{"docker"}},
				{ID: "writing", Provider: "gemini", Model: "gemini-pro", Priority: 100, MatchKeywords: []string{"essay"}},
			},
		}},
	}}
	r := New(cfg)
	first, ok := r.Route(Request{
		RequestedModel: "auto",
		Headers:        http.Header{"X-Session-Id": []string{"s1"}},
		Body:           []byte(`{"messages":[{"role":"user","content":"debug this docker build"}]}`),
	})
	if !ok || first.RoleID != "coding" {
		t.Fatalf("first decision = %+v, %v; want coding", first, ok)
	}
	second, ok := r.Route(Request{
		RequestedModel: "auto",
		Headers:        http.Header{"X-Session-Id": []string{"s1"}},
		Body:           []byte(`{"messages":[{"role":"user","content":"new task: write an essay outline"}]}`),
	})
	if !ok || second.RoleID != "writing" || second.Sticky {
		t.Fatalf("second decision = %+v, %v; want non-sticky writing switch", second, ok)
	}
	sessions := r.Sessions()
	if len(sessions) != 1 || sessions[0].RoleID != "writing" || sessions[0].SwitchCount != 1 {
		t.Fatalf("sessions = %+v, want switched writing session", sessions)
	}
}

func TestStickyRoleDoesNotSwitchWithoutExplicitIntent(t *testing.T) {
	cfg := &config.SDKConfig{AutoRouter: config.AutoRouterConfig{
		Enabled: true,
		Models: []config.AutoModelConfig{{
			Name:     "auto",
			Session:  config.AutoRouterSessionConfig{Enabled: true, TTL: "30m"},
			Fallback: config.AutoRouteTargetConfig{Provider: "claude", Model: "claude-sonnet"},
			Roles: []config.AutoRouterRoleConfig{
				{ID: "coding", Provider: "codex", Model: "gpt-5-codex", Priority: 100, MatchKeywords: []string{"docker"}},
				{ID: "writing", Provider: "gemini", Model: "gemini-pro", Priority: 100, MatchKeywords: []string{"essay"}},
			},
		}},
	}}
	r := New(cfg)
	r.Route(Request{
		RequestedModel: "auto",
		Headers:        http.Header{"X-Session-Id": []string{"s1"}},
		Body:           []byte(`{"messages":[{"role":"user","content":"debug this docker build"}]}`),
	})
	second, ok := r.Route(Request{
		RequestedModel: "auto",
		Headers:        http.Header{"X-Session-Id": []string{"s1"}},
		Body:           []byte(`{"messages":[{"role":"user","content":"write an essay outline"}]}`),
	})
	if !ok || second.RoleID != "coding" || !second.Sticky {
		t.Fatalf("second decision = %+v, %v; want sticky coding", second, ok)
	}
}

func TestExplicitSwitchRespectsMaxSwitches(t *testing.T) {
	cfg := &config.SDKConfig{AutoRouter: config.AutoRouterConfig{
		Enabled: true,
		Models: []config.AutoModelConfig{{
			Name: "auto",
			Session: config.AutoRouterSessionConfig{
				Enabled:     true,
				TTL:         "30m",
				MaxSwitches: 1,
			},
			Fallback: config.AutoRouteTargetConfig{Provider: "claude", Model: "claude-sonnet"},
			Roles: []config.AutoRouterRoleConfig{
				{ID: "coding", Provider: "codex", Model: "gpt-5-codex", Priority: 100, MatchKeywords: []string{"docker"}},
				{ID: "writing", Provider: "gemini", Model: "gemini-pro", Priority: 100, MatchKeywords: []string{"essay"}},
				{ID: "fast", Provider: "gemini", Model: "gemini-flash", Priority: 100, MatchKeywords: []string{"summary"}},
			},
		}},
	}}
	r := New(cfg)
	r.Route(Request{
		RequestedModel: "auto",
		Headers:        http.Header{"X-Session-Id": []string{"s1"}},
		Body:           []byte(`{"messages":[{"role":"user","content":"debug this docker build"}]}`),
	})
	r.Route(Request{
		RequestedModel: "auto",
		Headers:        http.Header{"X-Session-Id": []string{"s1"}},
		Body:           []byte(`{"messages":[{"role":"user","content":"new task: write an essay outline"}]}`),
	})
	third, ok := r.Route(Request{
		RequestedModel: "auto",
		Headers:        http.Header{"X-Session-Id": []string{"s1"}},
		Body:           []byte(`{"messages":[{"role":"user","content":"new task: summarize this"}]}`),
	})
	if !ok || third.RoleID != "writing" || !third.Sticky {
		t.Fatalf("third decision = %+v, %v; want sticky writing after switch limit", third, ok)
	}
}

func TestRouteSelectsCostFirstCandidateForLowComplexity(t *testing.T) {
	cfg := &config.SDKConfig{AutoRouter: config.AutoRouterConfig{
		Enabled: true,
		Models: []config.AutoModelConfig{{
			Name:     "auto",
			Policy:   config.AutoRouterPolicyConfig{Strategy: "cost-first"},
			Session:  config.AutoRouterSessionConfig{Enabled: false},
			Fallback: config.AutoRouteTargetConfig{Provider: "claude", Model: "claude-sonnet"},
			Roles: []config.AutoRouterRoleConfig{{
				ID:            "fast",
				Provider:      "claude",
				Model:         "claude-sonnet",
				MatchKeywords: []string{"hello"},
				Candidates: []config.AutoRouteCandidateConfig{
					{Provider: "codex", Model: "gpt-5-codex", CostTier: "high", CapabilityTier: "high", Priority: 100, MinComplexity: "high"},
					{Provider: "gemini", Model: "gemini-flash", CostTier: "low", CapabilityTier: "medium", Priority: 80, MaxComplexity: "medium"},
				},
			}},
		}},
	}}
	decision, ok := New(cfg).Route(Request{
		RequestedModel: "auto",
		Body:           []byte(`{"messages":[{"role":"user","content":"hello"}]}`),
	})
	if !ok {
		t.Fatal("Route() handled = false, want true")
	}
	if decision.Provider != "gemini" || decision.Model != "gemini-flash" || decision.Strategy != "cost-first" || decision.Complexity != "low" {
		t.Fatalf("decision = %+v, want low-cost gemini candidate", decision)
	}
}

func TestRouteSelectsQualityCandidateForHighComplexity(t *testing.T) {
	cfg := &config.SDKConfig{AutoRouter: config.AutoRouterConfig{
		Enabled: true,
		Models: []config.AutoModelConfig{{
			Name:     "auto",
			Policy:   config.AutoRouterPolicyConfig{Strategy: "quality-first"},
			Session:  config.AutoRouterSessionConfig{Enabled: false},
			Fallback: config.AutoRouteTargetConfig{Provider: "claude", Model: "claude-sonnet"},
			Roles: []config.AutoRouterRoleConfig{{
				ID:            "coding",
				Provider:      "gemini",
				Model:         "gemini-flash",
				MatchKeywords: []string{"debug"},
				Candidates: []config.AutoRouteCandidateConfig{
					{Provider: "gemini", Model: "gemini-flash", CostTier: "low", CapabilityTier: "medium", Priority: 80, MaxComplexity: "medium"},
					{Provider: "codex", Model: "gpt-5-codex", CostTier: "high", CapabilityTier: "high", Priority: 100, MinComplexity: "high"},
				},
			}},
		}},
	}}
	decision, ok := New(cfg).Route(Request{
		RequestedModel: "auto",
		Body:           []byte(`{"messages":[{"role":"user","content":"debug this repository architecture and failing test stack trace\nerror: build failed"}]}`),
	})
	if !ok {
		t.Fatal("Route() handled = false, want true")
	}
	if decision.Provider != "codex" || decision.Model != "gpt-5-codex" || decision.Strategy != "quality-first" || decision.Complexity != "high" {
		t.Fatalf("decision = %+v, want high-capability codex candidate", decision)
	}
}

func TestRouteCandidateSelectionKeepsLegacyRoleTarget(t *testing.T) {
	cfg := &config.SDKConfig{AutoRouter: config.AutoRouterConfig{
		Enabled: true,
		Models: []config.AutoModelConfig{{
			Name:     "auto",
			Session:  config.AutoRouterSessionConfig{Enabled: false},
			Fallback: config.AutoRouteTargetConfig{Provider: "claude", Model: "claude-sonnet"},
			Roles:    []config.AutoRouterRoleConfig{{ID: "fast", Provider: "gemini", Model: "gemini-flash", MatchKeywords: []string{"hello"}}},
		}},
	}}
	decision, ok := New(cfg).Route(Request{
		RequestedModel: "auto",
		Body:           []byte(`{"messages":[{"role":"user","content":"hello"}]}`),
	})
	if !ok {
		t.Fatal("Route() handled = false, want true")
	}
	if decision.Provider != "gemini" || decision.Model != "gemini-flash" || decision.Strategy != "balanced" {
		t.Fatalf("decision = %+v, want legacy role target", decision)
	}
}
