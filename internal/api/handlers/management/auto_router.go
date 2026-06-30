package management

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/autorouter"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
)

type autoRouterDryRunRequest struct {
	Model        string          `json:"model"`
	SourceFormat string          `json:"source_format"`
	Stream       bool            `json:"stream"`
	Headers      http.Header     `json:"headers"`
	Query        url.Values      `json:"query"`
	Body         json.RawMessage `json:"body"`
}

// GetAutoRouter returns the auto-router config block.
func (h *Handler) GetAutoRouter(c *gin.Context) {
	if h == nil || h.cfg == nil {
		c.JSON(200, gin.H{"auto-router": config.AutoRouterConfig{}})
		return
	}
	c.JSON(200, gin.H{"auto-router": h.cfg.AutoRouter})
}

// PutAutoRouter replaces the auto-router config block.
func (h *Handler) PutAutoRouter(c *gin.Context) {
	if h == nil || h.cfg == nil {
		c.JSON(500, gin.H{"error": "handler not initialized"})
		return
	}
	body, errRead := c.GetRawData()
	if errRead != nil {
		c.JSON(400, gin.H{"error": "failed to read body"})
		return
	}
	next, ok := decodeAutoRouterConfig(c, body)
	if !ok {
		return
	}
	h.mu.Lock()
	h.cfg.AutoRouter = next
	h.cfg.SanitizeAutoRouter()
	h.persistLocked(c)
	h.mu.Unlock()
}

// DeleteAutoRouter clears the auto-router config block.
func (h *Handler) DeleteAutoRouter(c *gin.Context) {
	if h == nil || h.cfg == nil {
		c.JSON(500, gin.H{"error": "handler not initialized"})
		return
	}
	h.mu.Lock()
	h.cfg.AutoRouter = config.AutoRouterConfig{}
	h.persistLocked(c)
	h.mu.Unlock()
}

// GetAutoRouterSessions returns the active sticky auto-router sessions.
func (h *Handler) GetAutoRouterSessions(c *gin.Context) {
	router := h.runtimeAutoRouter()
	if router == nil {
		c.JSON(http.StatusOK, gin.H{"sessions": []autorouter.SessionSnapshot{}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"sessions": router.Sessions()})
}

// DeleteAutoRouterSessions clears all active sticky auto-router sessions.
func (h *Handler) DeleteAutoRouterSessions(c *gin.Context) {
	router := h.runtimeAutoRouter()
	if router == nil {
		c.JSON(http.StatusOK, gin.H{"cleared": 0})
		return
	}
	c.JSON(http.StatusOK, gin.H{"cleared": router.ClearSessions()})
}

// PostAutoRouterDryRun previews deterministic routing without mutating sticky sessions.
func (h *Handler) PostAutoRouterDryRun(c *gin.Context) {
	router := h.runtimeAutoRouter()
	if router == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "auto-router is not initialized"})
		return
	}
	var req autoRouterDryRunRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = "auto"
	}
	sourceFormat := strings.TrimSpace(req.SourceFormat)
	if sourceFormat == "" {
		sourceFormat = "openai"
	}
	decision, handled := router.Preview(autorouter.Request{
		SourceFormat:   sourceFormat,
		RequestedModel: model,
		Stream:         req.Stream,
		Headers:        req.Headers,
		Query:          req.Query,
		Body:           []byte(req.Body),
	})
	c.JSON(http.StatusOK, gin.H{
		"handled":  handled,
		"decision": decision,
	})
}

func (h *Handler) runtimeAutoRouter() *autorouter.Router {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.autoRouter
}

func decodeAutoRouterConfig(c *gin.Context, data []byte) (config.AutoRouterConfig, bool) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err == nil {
		if value, exists := raw["auto-router"]; exists {
			var wrapped config.AutoRouterConfig
			if errWrapped := json.Unmarshal(value, &wrapped); errWrapped != nil {
				c.JSON(400, gin.H{"error": "invalid body"})
				return config.AutoRouterConfig{}, false
			}
			return sanitizeAutoRouterConfig(wrapped), true
		}
	}
	var direct config.AutoRouterConfig
	if err := json.Unmarshal(data, &direct); err != nil {
		c.JSON(400, gin.H{"error": "invalid body"})
		return config.AutoRouterConfig{}, false
	}
	return sanitizeAutoRouterConfig(direct), true
}

func sanitizeAutoRouterConfig(value config.AutoRouterConfig) config.AutoRouterConfig {
	cfg := &config.Config{}
	cfg.AutoRouter = value
	cfg.SanitizeAutoRouter()
	return cfg.AutoRouter
}
