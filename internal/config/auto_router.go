package config

import (
	"strings"
	"time"
)

const defaultAutoRouterSessionTTL = "30m"

// AutoRouterConfig configures client-visible auto models.
type AutoRouterConfig struct {
	Enabled     bool                         `yaml:"enabled" json:"enabled"`
	Models      []AutoModelConfig            `yaml:"models" json:"models"`
	RolePresets []AutoRouterRolePresetConfig `yaml:"role-presets,omitempty" json:"role-presets,omitempty"`
}

// AutoModelConfig describes one client-visible auto model.
type AutoModelConfig struct {
	Name        string                  `yaml:"name" json:"name"`
	Description string                  `yaml:"description,omitempty" json:"description,omitempty"`
	DefaultRole string                  `yaml:"default-role,omitempty" json:"default-role,omitempty"`
	Fallback    AutoRouteTargetConfig   `yaml:"fallback" json:"fallback"`
	Brain       AutoRouterBrainConfig   `yaml:"brain" json:"brain"`
	Session     AutoRouterSessionConfig `yaml:"session" json:"session"`
	Roles       []AutoRouterRoleConfig  `yaml:"roles" json:"roles"`
}

// AutoRouteTargetConfig points at a concrete provider/model target.
type AutoRouteTargetConfig struct {
	Provider string `yaml:"provider" json:"provider"`
	Model    string `yaml:"model" json:"model"`
}

// AutoRouterBrainConfig reserves the routing judge model used by later brain-backed decisions.
type AutoRouterBrainConfig struct {
	Provider       string  `yaml:"provider,omitempty" json:"provider,omitempty"`
	Model          string  `yaml:"model,omitempty" json:"model,omitempty"`
	PromptTemplate string  `yaml:"prompt-template,omitempty" json:"prompt-template,omitempty"`
	Temperature    float64 `yaml:"temperature,omitempty" json:"temperature,omitempty"`
	MaxTokens      int     `yaml:"max-tokens,omitempty" json:"max-tokens,omitempty"`
}

// AutoRouterSessionConfig controls sticky role bindings.
type AutoRouterSessionConfig struct {
	Enabled             bool     `yaml:"enabled" json:"enabled"`
	TTL                 string   `yaml:"ttl,omitempty" json:"ttl,omitempty"`
	SwitchThreshold     float64  `yaml:"switch-threshold,omitempty" json:"switch-threshold,omitempty"`
	MaxSwitches         int      `yaml:"max-switches,omitempty" json:"max-switches,omitempty"`
	SwitchKeywords      []string `yaml:"switch-keywords,omitempty" json:"switch-keywords,omitempty"`
	KeySources          []string `yaml:"key-sources,omitempty" json:"key-sources,omitempty"`
	FallbackHistoryHash *bool    `yaml:"fallback-history-hash,omitempty" json:"fallback-history-hash,omitempty"`
}

// AutoRouterRoleConfig maps an auto role to a concrete provider/model target.
type AutoRouterRoleConfig struct {
	ID             string   `yaml:"id" json:"id"`
	Name           string   `yaml:"name,omitempty" json:"name,omitempty"`
	Description    string   `yaml:"description,omitempty" json:"description,omitempty"`
	Provider       string   `yaml:"provider" json:"provider"`
	Model          string   `yaml:"model" json:"model"`
	CostTier       string   `yaml:"cost-tier,omitempty" json:"cost-tier,omitempty"`
	Priority       int      `yaml:"priority,omitempty" json:"priority,omitempty"`
	Strengths      []string `yaml:"strengths,omitempty" json:"strengths,omitempty"`
	MatchKeywords  []string `yaml:"match-keywords,omitempty" json:"match-keywords,omitempty"`
	PromptTemplate string   `yaml:"prompt-template,omitempty" json:"prompt-template,omitempty"`
	Disabled       bool     `yaml:"disabled,omitempty" json:"disabled,omitempty"`
}

// AutoRouterRolePresetConfig stores user-defined reusable role templates.
type AutoRouterRolePresetConfig struct {
	ID             string   `yaml:"id" json:"id"`
	Name           string   `yaml:"name,omitempty" json:"name,omitempty"`
	Description    string   `yaml:"description,omitempty" json:"description,omitempty"`
	CostTier       string   `yaml:"cost-tier,omitempty" json:"cost-tier,omitempty"`
	Priority       int      `yaml:"priority,omitempty" json:"priority,omitempty"`
	Strengths      []string `yaml:"strengths,omitempty" json:"strengths,omitempty"`
	MatchKeywords  []string `yaml:"match-keywords,omitempty" json:"match-keywords,omitempty"`
	PromptTemplate string   `yaml:"prompt-template,omitempty" json:"prompt-template,omitempty"`
}

// SanitizeAutoRouter normalizes auto-router configuration and removes unusable entries.
func (cfg *Config) SanitizeAutoRouter() {
	if cfg == nil {
		return
	}
	cfg.AutoRouter.Models = sanitizeAutoModels(cfg.AutoRouter.Models)
	cfg.AutoRouter.RolePresets = sanitizeAutoRolePresets(cfg.AutoRouter.RolePresets)
	if len(cfg.AutoRouter.Models) == 0 {
		cfg.AutoRouter.Enabled = false
	}
}

func sanitizeAutoModels(models []AutoModelConfig) []AutoModelConfig {
	out := make([]AutoModelConfig, 0, len(models))
	seen := make(map[string]struct{}, len(models))
	for _, model := range models {
		model.Name = strings.TrimSpace(model.Name)
		key := strings.ToLower(model.Name)
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		model.Description = strings.TrimSpace(model.Description)
		model.DefaultRole = strings.TrimSpace(model.DefaultRole)
		model.Fallback = sanitizeAutoTarget(model.Fallback)
		model.Brain.Provider = strings.ToLower(strings.TrimSpace(model.Brain.Provider))
		model.Brain.Model = strings.TrimSpace(model.Brain.Model)
		model.Brain.PromptTemplate = strings.TrimSpace(model.Brain.PromptTemplate)
		if model.Brain.MaxTokens < 0 {
			model.Brain.MaxTokens = 0
		}
		model.Session = sanitizeAutoSession(model.Session)
		model.Roles = sanitizeAutoRoles(model.Roles)
		out = append(out, model)
	}
	return out
}

func sanitizeAutoTarget(target AutoRouteTargetConfig) AutoRouteTargetConfig {
	target.Provider = strings.ToLower(strings.TrimSpace(target.Provider))
	target.Model = strings.TrimSpace(target.Model)
	return target
}

func sanitizeAutoSession(session AutoRouterSessionConfig) AutoRouterSessionConfig {
	session.TTL = strings.TrimSpace(session.TTL)
	if session.TTL == "" {
		session.TTL = defaultAutoRouterSessionTTL
	} else if _, err := time.ParseDuration(session.TTL); err != nil {
		session.TTL = defaultAutoRouterSessionTTL
	}
	if session.SwitchThreshold < 0 {
		session.SwitchThreshold = 0
	}
	if session.SwitchThreshold > 1 {
		session.SwitchThreshold = 1
	}
	if session.MaxSwitches < 0 {
		session.MaxSwitches = 0
	}
	session.SwitchKeywords = trimUniqueStrings(session.SwitchKeywords, true)
	session.KeySources = trimUniqueStrings(session.KeySources, false)
	return session
}

func sanitizeAutoRoles(roles []AutoRouterRoleConfig) []AutoRouterRoleConfig {
	out := make([]AutoRouterRoleConfig, 0, len(roles))
	seen := make(map[string]struct{}, len(roles))
	for _, role := range roles {
		role.ID = strings.TrimSpace(role.ID)
		key := strings.ToLower(role.ID)
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		role.Name = strings.TrimSpace(role.Name)
		role.Description = strings.TrimSpace(role.Description)
		role.Provider = strings.ToLower(strings.TrimSpace(role.Provider))
		role.Model = strings.TrimSpace(role.Model)
		role.CostTier = strings.ToLower(strings.TrimSpace(role.CostTier))
		role.Strengths = trimUniqueStrings(role.Strengths, false)
		role.MatchKeywords = trimUniqueStrings(role.MatchKeywords, true)
		role.PromptTemplate = strings.TrimSpace(role.PromptTemplate)
		if role.Provider == "" || role.Model == "" {
			continue
		}
		out = append(out, role)
	}
	return out
}

func sanitizeAutoRolePresets(presets []AutoRouterRolePresetConfig) []AutoRouterRolePresetConfig {
	out := make([]AutoRouterRolePresetConfig, 0, len(presets))
	seen := make(map[string]struct{}, len(presets))
	for _, preset := range presets {
		preset.ID = strings.TrimSpace(preset.ID)
		key := strings.ToLower(preset.ID)
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		preset.Name = strings.TrimSpace(preset.Name)
		preset.Description = strings.TrimSpace(preset.Description)
		preset.CostTier = strings.ToLower(strings.TrimSpace(preset.CostTier))
		preset.Strengths = trimUniqueStrings(preset.Strengths, false)
		preset.MatchKeywords = trimUniqueStrings(preset.MatchKeywords, true)
		preset.PromptTemplate = strings.TrimSpace(preset.PromptTemplate)
		out = append(out, preset)
	}
	return out
}

func trimUniqueStrings(values []string, lower bool) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if lower {
			value = strings.ToLower(value)
		}
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
