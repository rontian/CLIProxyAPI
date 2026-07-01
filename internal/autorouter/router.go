package autorouter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/thinking"
	coreexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	"github.com/tidwall/gjson"
)

// Request contains the inbound context needed to choose an auto route.
type Request struct {
	SourceFormat   string
	RequestedModel string
	Stream         bool
	Headers        http.Header
	Query          url.Values
	Body           []byte
	Metadata       map[string]any
}

// Decision is the selected concrete model target.
type Decision struct {
	Provider       string  `json:"provider"`
	Model          string  `json:"model"`
	RoleID         string  `json:"role_id"`
	Reason         string  `json:"reason,omitempty"`
	PromptTemplate string  `json:"prompt_template,omitempty"`
	Confidence     float64 `json:"confidence,omitempty"`
	Brain          bool    `json:"brain"`
	Sticky         bool    `json:"sticky"`
	Strategy       string  `json:"strategy,omitempty"`
	Complexity     string  `json:"complexity,omitempty"`
}

// BrainRequest is passed to an optional model-backed judge.
type BrainRequest struct {
	AutoModel       config.AutoModelConfig
	Request         Request
	RequestedSuffix string
}

// BrainDecision is the judge's structured routing decision.
type BrainDecision struct {
	RoleID     string
	Provider   string
	Model      string
	Confidence float64
	Reason     string
}

// JudgeFunc decides an auto route from model-backed context.
type JudgeFunc func(context.Context, BrainRequest) (BrainDecision, error)

type sessionBinding struct {
	autoModel      string
	roleID         string
	provider       string
	model          string
	reason         string
	promptTemplate string
	switchCount    int
	expiresAt      time.Time
}

// SessionSnapshot describes a currently active sticky auto-router binding.
type SessionSnapshot struct {
	Key         string    `json:"key"`
	AutoModel   string    `json:"auto_model"`
	RoleID      string    `json:"role_id"`
	Provider    string    `json:"provider"`
	Model       string    `json:"model"`
	Reason      string    `json:"reason,omitempty"`
	SwitchCount int       `json:"switch_count"`
	ExpiresAt   time.Time `json:"expires_at"`
}

type routeState struct {
	autoModel       config.AutoModelConfig
	requestedSuffix string
	sessionKey      string
	stickyDecision  Decision
	priorBinding    *sessionBinding
}

// Router implements stable first-version routing for configured auto models.
type Router struct {
	mu       sync.Mutex
	cfg      config.AutoRouterConfig
	sessions map[string]sessionBinding
	now      func() time.Time
}

// New creates an auto router for cfg.
func New(cfg *config.SDKConfig) *Router {
	r := &Router{
		sessions: make(map[string]sessionBinding),
		now:      time.Now,
	}
	r.UpdateConfig(cfg)
	return r
}

// UpdateConfig replaces the runtime routing config and drops stale bindings.
func (r *Router) UpdateConfig(cfg *config.SDKConfig) {
	if r == nil {
		return
	}
	var next config.AutoRouterConfig
	if cfg != nil {
		next = cfg.AutoRouter
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cfg = next
	r.pruneLocked(r.now())
}

// Route chooses a concrete provider/model for an auto request.
func (r *Router) Route(req Request) (Decision, bool) {
	return r.RouteWithJudge(context.Background(), req, nil)
}

// Preview chooses a route without creating or extending sticky session bindings.
func (r *Router) Preview(req Request) (Decision, bool) {
	if r == nil {
		return Decision{}, false
	}
	state, ok := r.prepareRoute(req)
	if !ok {
		return Decision{}, false
	}
	if state.stickyDecision.Provider != "" {
		return state.stickyDecision, true
	}
	role, reason := selectRole(state.autoModel, req)
	decision := decisionFromRoleOrFallback(state.autoModel, role, reason, state.requestedSuffix, req)
	if state.priorBinding != nil {
		decision = switchDecisionOrSticky(state.autoModel, *state.priorBinding, decision, state.requestedSuffix)
	}
	return decision, decision.Provider != "" && decision.Model != ""
}

// Sessions returns the active sticky session bindings.
func (r *Router) Sessions() []SessionSnapshot {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pruneLocked(r.now())
	sessions := make([]SessionSnapshot, 0, len(r.sessions))
	for key, binding := range r.sessions {
		sessions = append(sessions, SessionSnapshot{
			Key:         key,
			AutoModel:   binding.autoModel,
			RoleID:      binding.roleID,
			Provider:    binding.provider,
			Model:       binding.model,
			Reason:      binding.reason,
			SwitchCount: binding.switchCount,
			ExpiresAt:   binding.expiresAt,
		})
	}
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].ExpiresAt.Before(sessions[j].ExpiresAt)
	})
	return sessions
}

// ClearSessions removes all sticky session bindings and returns the removed count.
func (r *Router) ClearSessions() int {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	count := len(r.sessions)
	r.sessions = make(map[string]sessionBinding)
	return count
}

// RouteWithJudge chooses a route and optionally asks a model-backed judge before fallback matching.
func (r *Router) RouteWithJudge(ctx context.Context, req Request, judge JudgeFunc) (Decision, bool) {
	if r == nil {
		return Decision{}, false
	}
	state, ok := r.prepareRoute(req)
	if !ok {
		return Decision{}, false
	}
	if state.stickyDecision.Provider != "" {
		return state.stickyDecision, true
	}

	if decision, okJudge := r.judgeDecision(ctx, state.autoModel, req, state.requestedSuffix, judge); okJudge {
		decision = switchDecisionOrSticky(state.autoModel, bindingValue(state.priorBinding), decision, state.requestedSuffix)
		if !decision.Sticky {
			r.storeSession(state.autoModel, state.sessionKey, decision, state.priorBinding)
		}
		return decision, true
	}

	role, reason := selectRole(state.autoModel, req)
	decision := decisionFromRoleOrFallback(state.autoModel, role, reason, state.requestedSuffix, req)
	if decision.Provider == "" || decision.Model == "" {
		return Decision{}, false
	}
	decision = switchDecisionOrSticky(state.autoModel, bindingValue(state.priorBinding), decision, state.requestedSuffix)
	if !decision.Sticky {
		r.storeSession(state.autoModel, state.sessionKey, decision, state.priorBinding)
	}
	return decision, true
}

func (r *Router) prepareRoute(req Request) (routeState, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	r.pruneLocked(now)
	if !r.cfg.Enabled {
		return routeState{}, false
	}

	autoModel, requestedSuffix, ok := r.findAutoModelLocked(req.RequestedModel)
	if !ok {
		return routeState{}, false
	}

	sessionKey := sessionKey(req, autoModel)
	state := routeState{
		autoModel:       autoModel,
		requestedSuffix: requestedSuffix,
		sessionKey:      sessionKey,
	}
	if sessionKey != "" && autoModel.Session.Enabled {
		if binding, exists := r.sessions[sessionKey]; exists && binding.autoModel == autoModel.Name && now.Before(binding.expiresAt) {
			state.priorBinding = cloneBinding(binding)
			if shouldEvaluateSwitch(autoModel.Session, req, binding) {
				return state, true
			}
			state.stickyDecision = Decision{
				Provider:       binding.provider,
				Model:          withRequestedSuffix(binding.model, requestedSuffix),
				RoleID:         binding.roleID,
				Reason:         binding.reason,
				PromptTemplate: binding.promptTemplate,
				Sticky:         true,
			}
			return state, true
		}
	}
	return state, true
}

func (r *Router) judgeDecision(ctx context.Context, autoModel config.AutoModelConfig, req Request, requestedSuffix string, judge JudgeFunc) (Decision, bool) {
	if judge == nil || strings.TrimSpace(autoModel.Brain.Provider) == "" || strings.TrimSpace(autoModel.Brain.Model) == "" {
		return Decision{}, false
	}
	brainDecision, err := judge(ctx, BrainRequest{
		AutoModel:       autoModel,
		Request:         req,
		RequestedSuffix: requestedSuffix,
	})
	if err != nil {
		return Decision{}, false
	}
	decision, ok := decisionFromBrain(autoModel, brainDecision, requestedSuffix, req)
	return decision, ok
}

func (r *Router) storeSession(autoModel config.AutoModelConfig, sessionKey string, decision Decision, prior *sessionBinding) {
	if r == nil || sessionKey == "" || !autoModel.Session.Enabled || decision.Provider == "" || decision.Model == "" {
		return
	}
	ttl := autoSessionTTL(autoModel.Session)
	switchCount := 0
	if prior != nil {
		switchCount = prior.switchCount
		if !sameBindingDecision(*prior, decision) {
			switchCount++
		}
	}
	r.mu.Lock()
	r.sessions[sessionKey] = sessionBinding{
		autoModel:      autoModel.Name,
		roleID:         decision.RoleID,
		provider:       decision.Provider,
		model:          strings.TrimSpace(roleModelForBinding(decision.Model)),
		reason:         decision.Reason,
		promptTemplate: decision.PromptTemplate,
		switchCount:    switchCount,
		expiresAt:      r.now().Add(ttl),
	}
	r.mu.Unlock()
}

func (r *Router) findAutoModelLocked(requestedModel string) (config.AutoModelConfig, string, bool) {
	parsed := thinking.ParseSuffix(strings.TrimSpace(requestedModel))
	base := strings.ToLower(strings.TrimSpace(parsed.ModelName))
	if base == "" {
		return config.AutoModelConfig{}, "", false
	}
	for _, model := range r.cfg.Models {
		if strings.ToLower(model.Name) == base {
			return model, parsed.RawSuffix, true
		}
	}
	return config.AutoModelConfig{}, "", false
}

func (r *Router) pruneLocked(now time.Time) {
	for key, binding := range r.sessions {
		if !binding.expiresAt.IsZero() && now.After(binding.expiresAt) {
			delete(r.sessions, key)
		}
	}
}

func selectRole(model config.AutoModelConfig, req Request) (config.AutoRouterRoleConfig, string) {
	roles := make([]config.AutoRouterRoleConfig, 0, len(model.Roles))
	for _, role := range model.Roles {
		if !role.Disabled {
			roles = append(roles, role)
		}
	}
	sort.SliceStable(roles, func(i, j int) bool {
		return roles[i].Priority > roles[j].Priority
	})
	text := strings.ToLower(requestText(req.Body))
	for _, role := range roles {
		for _, keyword := range role.MatchKeywords {
			if keyword != "" && strings.Contains(text, keyword) {
				return role, fmt.Sprintf("matched keyword %q", keyword)
			}
		}
	}
	if model.DefaultRole != "" {
		for _, role := range roles {
			if strings.EqualFold(role.ID, model.DefaultRole) {
				return role, "selected default role"
			}
		}
	}
	return config.AutoRouterRoleConfig{}, "selected fallback target"
}

func decisionFromRoleOrFallback(model config.AutoModelConfig, role config.AutoRouterRoleConfig, reason, requestedSuffix string, req Request) Decision {
	if role.ID != "" {
		target, complexity := selectRoleTarget(model.Policy, role, req)
		return Decision{
			Provider:       target.Provider,
			Model:          withRequestedSuffix(target.Model, requestedSuffix),
			RoleID:         role.ID,
			Reason:         reasonWithTarget(reason, target),
			PromptTemplate: strings.TrimSpace(role.PromptTemplate),
			Confidence:     1,
			Strategy:       autoPolicyStrategy(model.Policy),
			Complexity:     complexity,
		}
	}
	return Decision{
		Provider:   model.Fallback.Provider,
		Model:      withRequestedSuffix(model.Fallback.Model, requestedSuffix),
		RoleID:     "fallback",
		Reason:     reason,
		Confidence: 0.5,
		Strategy:   autoPolicyStrategy(model.Policy),
		Complexity: requestComplexity(req.Body),
	}
}

func decisionFromBrain(model config.AutoModelConfig, brain BrainDecision, requestedSuffix string, req Request) (Decision, bool) {
	roleID := strings.TrimSpace(brain.RoleID)
	if roleID == "" {
		return Decision{}, false
	}
	reason := strings.TrimSpace(brain.Reason)
	if reason == "" {
		reason = "selected by brain judge"
	}
	if strings.EqualFold(roleID, "fallback") {
		if model.Fallback.Provider == "" || model.Fallback.Model == "" {
			return Decision{}, false
		}
		return Decision{
			Provider:   model.Fallback.Provider,
			Model:      withRequestedSuffix(model.Fallback.Model, requestedSuffix),
			RoleID:     "fallback",
			Reason:     reason,
			Confidence: brain.Confidence,
			Brain:      true,
		}, true
	}
	for _, role := range model.Roles {
		if role.Disabled || !strings.EqualFold(role.ID, roleID) {
			continue
		}
		target, complexity := selectRoleTarget(model.Policy, role, req)
		return Decision{
			Provider:       target.Provider,
			Model:          withRequestedSuffix(target.Model, requestedSuffix),
			RoleID:         role.ID,
			Reason:         reasonWithTarget(reason, target),
			PromptTemplate: strings.TrimSpace(role.PromptTemplate),
			Confidence:     brain.Confidence,
			Brain:          true,
			Strategy:       autoPolicyStrategy(model.Policy),
			Complexity:     complexity,
		}, true
	}
	return Decision{}, false
}

func selectRoleTarget(policy config.AutoRouterPolicyConfig, role config.AutoRouterRoleConfig, req Request) (config.AutoRouteCandidateConfig, string) {
	complexity := requestComplexity(req.Body)
	candidates := roleCandidates(role)
	if len(candidates) == 0 {
		return config.AutoRouteCandidateConfig{}, complexity
	}
	strategy := autoPolicyStrategy(policy)
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidateScore(strategy, complexity, candidates[i]) > candidateScore(strategy, complexity, candidates[j])
	})
	return candidates[0], complexity
}

func roleCandidates(role config.AutoRouterRoleConfig) []config.AutoRouteCandidateConfig {
	candidates := make([]config.AutoRouteCandidateConfig, 0, len(role.Candidates)+1)
	for _, candidate := range role.Candidates {
		if candidate.Disabled || candidate.Provider == "" || candidate.Model == "" {
			continue
		}
		candidates = append(candidates, candidate)
	}
	if role.Provider != "" && role.Model != "" {
		exists := false
		for _, candidate := range candidates {
			if strings.EqualFold(candidate.Provider, role.Provider) && strings.EqualFold(candidate.Model, role.Model) {
				exists = true
				break
			}
		}
		if !exists {
			candidates = append(candidates, config.AutoRouteCandidateConfig{
				Provider:       role.Provider,
				Model:          role.Model,
				CostTier:       role.CostTier,
				CapabilityTier: "medium",
				Priority:       role.Priority,
			})
		}
	}
	return candidates
}

func candidateScore(strategy, complexity string, candidate config.AutoRouteCandidateConfig) int {
	score := candidate.Priority
	score += complexityFitScore(complexity, candidate)
	cost := tierValue(candidate.CostTier)
	capability := tierValue(candidate.CapabilityTier)
	switch strategy {
	case "cost-first":
		score += (2 - cost) * 35
		score += capability * 8
	case "quality-first":
		score += capability * 35
		if complexity == "low" {
			score += (2 - cost) * 8
		}
	default:
		score += capability * 20
		score += (2 - cost) * 16
	}
	if complexity == "high" {
		score += capability * 20
	}
	if complexity == "low" {
		score += (2 - cost) * 12
	}
	return score
}

func complexityFitScore(complexity string, candidate config.AutoRouteCandidateConfig) int {
	level := complexityValue(complexity)
	minLevel := complexityValue(candidate.MinComplexity)
	maxLevel := complexityValue(candidate.MaxComplexity)
	if candidate.MinComplexity != "" && level < minLevel {
		return -120
	}
	if candidate.MaxComplexity != "" && level > maxLevel {
		return -120
	}
	return 40
}

func autoPolicyStrategy(policy config.AutoRouterPolicyConfig) string {
	switch strings.ToLower(strings.TrimSpace(policy.Strategy)) {
	case "cost-first", "quality-first":
		return strings.ToLower(strings.TrimSpace(policy.Strategy))
	default:
		return "balanced"
	}
}

func tierValue(value string) int {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "high":
		return 2
	case "medium":
		return 1
	case "low":
		return 0
	default:
		return 1
	}
}

func complexityValue(value string) int {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "high":
		return 2
	case "medium":
		return 1
	case "low":
		return 0
	default:
		return 1
	}
}

func requestComplexity(body []byte) string {
	text := strings.ToLower(requestText(body))
	words := strings.Fields(text)
	score := 0
	if len(words) > 180 {
		score += 2
	} else if len(words) > 60 {
		score++
	}
	highSignals := []string{
		"architecture", "refactor", "debug", "stack trace", "failing test", "implement",
		"repository", "repo", "migration", "performance", "security", "并发", "架构", "重构", "调试", "实现",
	}
	for _, signal := range highSignals {
		if strings.Contains(text, signal) {
			score += 2
			break
		}
	}
	mediumSignals := []string{
		"explain", "compare", "analyze", "summarize", "translate", "review", "分析", "总结", "翻译", "解释",
	}
	for _, signal := range mediumSignals {
		if strings.Contains(text, signal) {
			score++
			break
		}
	}
	if strings.Contains(text, "```") || strings.Contains(text, "error:") || strings.Contains(text, "panic:") {
		score += 2
	}
	switch {
	case score >= 3:
		return "high"
	case score >= 1:
		return "medium"
	default:
		return "low"
	}
}

func reasonWithTarget(reason string, target config.AutoRouteCandidateConfig) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "selected role target"
	}
	if target.Provider == "" || target.Model == "" {
		return reason
	}
	return fmt.Sprintf("%s; selected candidate %s/%s", reason, target.Provider, target.Model)
}

func switchDecisionOrSticky(model config.AutoModelConfig, prior sessionBinding, decision Decision, requestedSuffix string) Decision {
	if prior.provider == "" || decision.Provider == "" || decision.Model == "" {
		return decision
	}
	if sameBindingDecision(prior, decision) {
		return stickyDecisionFromBinding(prior, requestedSuffix)
	}
	if !candidateCanSwitch(model.Session, prior, decision) {
		return stickyDecisionFromBinding(prior, requestedSuffix)
	}
	return decision
}

func candidateCanSwitch(session config.AutoRouterSessionConfig, prior sessionBinding, decision Decision) bool {
	if session.MaxSwitches > 0 && prior.switchCount >= session.MaxSwitches {
		return false
	}
	threshold := session.SwitchThreshold
	if threshold <= 0 {
		threshold = 0.85
	}
	confidence := decision.Confidence
	if confidence <= 0 && !decision.Brain {
		confidence = 1
	}
	return confidence >= threshold
}

func sameBindingDecision(binding sessionBinding, decision Decision) bool {
	return strings.EqualFold(binding.roleID, decision.RoleID) &&
		strings.EqualFold(binding.provider, decision.Provider) &&
		strings.EqualFold(binding.model, roleModelForBinding(decision.Model))
}

func stickyDecisionFromBinding(binding sessionBinding, requestedSuffix string) Decision {
	return Decision{
		Provider:       binding.provider,
		Model:          withRequestedSuffix(binding.model, requestedSuffix),
		RoleID:         binding.roleID,
		Reason:         binding.reason,
		PromptTemplate: binding.promptTemplate,
		Sticky:         true,
	}
}

func shouldEvaluateSwitch(session config.AutoRouterSessionConfig, req Request, binding sessionBinding) bool {
	if session.MaxSwitches > 0 && binding.switchCount >= session.MaxSwitches {
		return false
	}
	if boolSignal(req.Headers.Get("X-Auto-Route-Reset")) || boolSignal(req.Headers.Get("X-Auto-Router-Reset")) {
		return true
	}
	if boolSignal(req.Query.Get("auto_route_reset")) || boolSignal(req.Query.Get("auto_router_reset")) {
		return true
	}
	if gjson.GetBytes(req.Body, "metadata.auto_route_reset").Bool() || gjson.GetBytes(req.Body, "metadata.auto_router_reset").Bool() {
		return true
	}
	text := strings.ToLower(requestText(req.Body))
	if text == "" {
		return false
	}
	for _, keyword := range switchKeywords(session) {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func switchKeywords(session config.AutoRouterSessionConfig) []string {
	if len(session.SwitchKeywords) > 0 {
		return session.SwitchKeywords
	}
	return []string{
		"switch to",
		"change to",
		"route to",
		"handoff to",
		"new task",
		"new topic",
		"different task",
		"different topic",
		"separate question",
		"now use",
		"forget the previous task",
	}
}

func boolSignal(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func cloneBinding(binding sessionBinding) *sessionBinding {
	cloned := binding
	return &cloned
}

func bindingValue(binding *sessionBinding) sessionBinding {
	if binding == nil {
		return sessionBinding{}
	}
	return *binding
}

func roleModelForBinding(model string) string {
	return strings.TrimSpace(thinking.ParseSuffix(model).ModelName)
}

func withRequestedSuffix(model, suffix string) string {
	model = strings.TrimSpace(model)
	suffix = strings.TrimSpace(suffix)
	if model == "" || suffix == "" || thinking.ParseSuffix(model).HasSuffix {
		return model
	}
	return fmt.Sprintf("%s(%s)", model, suffix)
}

func autoSessionTTL(session config.AutoRouterSessionConfig) time.Duration {
	if ttl, err := time.ParseDuration(strings.TrimSpace(session.TTL)); err == nil && ttl > 0 {
		return ttl
	}
	return 30 * time.Minute
}

func sessionKey(req Request, model config.AutoModelConfig) string {
	for _, source := range model.Session.KeySources {
		if key := sessionKeyFromSource(req, source); key != "" {
			return key
		}
	}
	if key := metadataString(req.Metadata, coreexecutor.ExecutionSessionMetadataKey); key != "" {
		return "execution:" + key
	}
	for _, header := range []string{"X-Session-ID", "Session-Id", "Session_id", "X-Client-Request-Id"} {
		if key := strings.TrimSpace(req.Headers.Get(header)); key != "" {
			return "header:" + strings.ToLower(header) + ":" + key
		}
	}
	for _, path := range []string{"session_id", "conversation_id", "metadata.session_id", "metadata.conversation_id", "metadata.user_id"} {
		if key := strings.TrimSpace(gjson.GetBytes(req.Body, path).String()); key != "" {
			return "body:" + path + ":" + key
		}
	}
	if model.Session.FallbackHistoryHash == nil || *model.Session.FallbackHistoryHash {
		if key := historyHash(req.Body); key != "" {
			return "history:" + key
		}
	}
	return ""
}

func sessionKeyFromSource(req Request, source string) string {
	source = strings.ToLower(strings.TrimSpace(source))
	switch {
	case source == "metadata.execution_session_id":
		if key := metadataString(req.Metadata, coreexecutor.ExecutionSessionMetadataKey); key != "" {
			return "execution:" + key
		}
	case strings.HasPrefix(source, "headers."):
		name := strings.TrimSpace(strings.TrimPrefix(source, "headers."))
		if key := strings.TrimSpace(req.Headers.Get(name)); key != "" {
			return "header:" + name + ":" + key
		}
	case strings.HasPrefix(source, "query."):
		name := strings.TrimSpace(strings.TrimPrefix(source, "query."))
		if key := strings.TrimSpace(req.Query.Get(name)); key != "" {
			return "query:" + name + ":" + key
		}
	case strings.HasPrefix(source, "body."):
		path := strings.TrimSpace(strings.TrimPrefix(source, "body."))
		if key := strings.TrimSpace(gjson.GetBytes(req.Body, path).String()); key != "" {
			return "body:" + path + ":" + key
		}
	case source == "history-hash":
		if key := historyHash(req.Body); key != "" {
			return "history:" + key
		}
	}
	return ""
}

func metadataString(meta map[string]any, key string) string {
	if meta == nil {
		return ""
	}
	switch value := meta[key].(type) {
	case string:
		return strings.TrimSpace(value)
	case []byte:
		return strings.TrimSpace(string(value))
	default:
		return ""
	}
}

func historyHash(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	source := gjson.GetBytes(body, "messages")
	if !source.Exists() {
		source = gjson.GetBytes(body, "input")
	}
	if !source.Exists() {
		return ""
	}
	values := source.Array()
	if len(values) == 0 {
		return ""
	}
	limit := len(values)
	if limit > 4 {
		limit = 4
	}
	var b strings.Builder
	for i := 0; i < limit; i++ {
		b.WriteString(values[i].Raw)
		b.WriteByte('\n')
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])[:24]
}

func requestText(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var parts []string
	collectText(gjson.GetBytes(body, "messages"), &parts)
	collectText(gjson.GetBytes(body, "input"), &parts)
	if len(parts) == 0 {
		return string(body)
	}
	return strings.Join(parts, "\n")
}

func collectText(value gjson.Result, parts *[]string) {
	if !value.Exists() {
		return
	}
	switch {
	case value.IsArray():
		for _, child := range value.Array() {
			collectText(child.Get("content"), parts)
			collectText(child.Get("text"), parts)
		}
	case value.IsObject():
		collectText(value.Get("content"), parts)
		collectText(value.Get("text"), parts)
	case value.Type == gjson.String:
		text := strings.TrimSpace(value.String())
		if text != "" {
			*parts = append(*parts, text)
		}
	}
}
