package management

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/autorouter"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"github.com/tidwall/gjson"
)

func TestPutAutoRouterUpdatesConfig(t *testing.T) {
	gin.SetMode(gin.TestMode)
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := WriteConfig(configPath, []byte("api-keys: [test]\n")); err != nil {
		t.Fatalf("WriteConfig() error = %v", err)
	}
	h := NewHandler(&config.Config{}, configPath, nil)

	body := []byte(`{"auto-router":{"enabled":true,"role-presets":[{"id":" Debug ","name":" Debug Preset ","match-keywords":[" Error "]}],"models":[{"name":" auto ","fallback":{"provider":" Claude ","model":" sonnet "},"roles":[{"id":" coding ","provider":" Codex ","model":" gpt-5-codex "}]}]}}`)
	resp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(resp)
	c.Request = httptest.NewRequest(http.MethodPut, "/v0/management/auto-router", bytes.NewReader(body))

	h.PutAutoRouter(c)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.Code, resp.Body.String())
	}
	if !h.cfg.AutoRouter.Enabled || h.cfg.AutoRouter.Models[0].Name != "auto" {
		t.Fatalf("auto-router = %+v, want sanitized enabled auto model", h.cfg.AutoRouter)
	}
	if got := h.cfg.AutoRouter.Models[0].Roles[0].Provider; got != "codex" {
		t.Fatalf("role provider = %q, want codex", got)
	}
	if got := h.cfg.AutoRouter.RolePresets[0].MatchKeywords[0]; got != "error" {
		t.Fatalf("role preset keyword = %q, want error", got)
	}
}

func TestGetAutoRouterReturnsWrappedConfig(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewHandler(&config.Config{SDKConfig: config.SDKConfig{AutoRouter: config.AutoRouterConfig{
		Enabled:     true,
		Models:      []config.AutoModelConfig{{Name: "auto"}},
		RolePresets: []config.AutoRouterRolePresetConfig{{ID: "custom-debug", Name: "Custom Debug"}},
	}}}, "", nil)
	resp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(resp)
	c.Request = httptest.NewRequest(http.MethodGet, "/v0/management/auto-router", nil)

	h.GetAutoRouter(c)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d", resp.Code)
	}
	if got := gjson.GetBytes(resp.Body.Bytes(), "auto-router.models.0.name").String(); got != "auto" {
		t.Fatalf("model name = %q, want auto; body=%s", got, resp.Body.String())
	}
	if got := gjson.GetBytes(resp.Body.Bytes(), "auto-router.role-presets.0.id").String(); got != "custom-debug" {
		t.Fatalf("role preset id = %q, want custom-debug; body=%s", got, resp.Body.String())
	}
}

func TestAutoRouterSessionsCanBeListedAndCleared(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{SDKConfig: config.SDKConfig{AutoRouter: config.AutoRouterConfig{
		Enabled: true,
		Models: []config.AutoModelConfig{{
			Name:     "auto",
			Session:  config.AutoRouterSessionConfig{Enabled: true, TTL: "30m"},
			Fallback: config.AutoRouteTargetConfig{Provider: "claude", Model: "sonnet"},
			Roles:    []config.AutoRouterRoleConfig{{ID: "coding", Provider: "codex", Model: "gpt-5-codex", MatchKeywords: []string{"docker"}}},
		}},
	}}}
	router := autorouter.New(&cfg.SDKConfig)
	router.Route(autorouter.Request{
		RequestedModel: "auto",
		Headers:        http.Header{"X-Session-Id": []string{"s1"}},
		Body:           []byte(`{"messages":[{"role":"user","content":"docker fails"}]}`),
	})
	h := NewHandler(cfg, "", nil)
	h.SetAutoRouter(router)

	resp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(resp)
	c.Request = httptest.NewRequest(http.MethodGet, "/v0/management/auto-router/sessions", nil)
	h.GetAutoRouterSessions(c)
	if resp.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", resp.Code, resp.Body.String())
	}
	if got := gjson.GetBytes(resp.Body.Bytes(), "sessions.0.role_id").String(); got != "coding" {
		t.Fatalf("sessions.0.role_id = %q, want coding; body=%s", got, resp.Body.String())
	}

	resp = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(resp)
	c.Request = httptest.NewRequest(http.MethodDelete, "/v0/management/auto-router/sessions", nil)
	h.DeleteAutoRouterSessions(c)
	if got := gjson.GetBytes(resp.Body.Bytes(), "cleared").Int(); got != 1 {
		t.Fatalf("cleared = %d, want 1; body=%s", got, resp.Body.String())
	}
}

func TestPostAutoRouterDryRunPreviewsWithoutSessionMutation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{SDKConfig: config.SDKConfig{AutoRouter: config.AutoRouterConfig{
		Enabled: true,
		Models: []config.AutoModelConfig{{
			Name:     "auto",
			Session:  config.AutoRouterSessionConfig{Enabled: true, TTL: "30m"},
			Fallback: config.AutoRouteTargetConfig{Provider: "claude", Model: "sonnet"},
			Roles:    []config.AutoRouterRoleConfig{{ID: "coding", Provider: "codex", Model: "gpt-5-codex", MatchKeywords: []string{"docker"}}},
		}},
	}}}
	router := autorouter.New(&cfg.SDKConfig)
	h := NewHandler(cfg, "", nil)
	h.SetAutoRouter(router)

	body := []byte(`{"model":"auto","headers":{"X-Session-Id":["dry"]},"body":{"messages":[{"role":"user","content":"docker fails"}]}}`)
	resp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(resp)
	c.Request = httptest.NewRequest(http.MethodPost, "/v0/management/auto-router/dry-run", bytes.NewReader(body))
	h.PostAutoRouterDryRun(c)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.Code, resp.Body.String())
	}
	if !gjson.GetBytes(resp.Body.Bytes(), "handled").Bool() {
		t.Fatalf("handled = false; body=%s", resp.Body.String())
	}
	if got := gjson.GetBytes(resp.Body.Bytes(), "decision.role_id").String(); got != "coding" {
		t.Fatalf("decision.role_id = %q, want coding; body=%s", got, resp.Body.String())
	}
	if sessions := router.Sessions(); len(sessions) != 0 {
		t.Fatalf("dry-run sessions = %+v, want no mutation", sessions)
	}
}
