package main

/*
#include <stdint.h>
#include <stdlib.h>

typedef struct {
	void* ptr;
	size_t len;
} cliproxy_buffer;

typedef int (*cliproxy_host_call_fn)(void*, const char*, const uint8_t*, size_t, cliproxy_buffer*);
typedef void (*cliproxy_host_free_fn)(void*, size_t);

typedef struct {
	uint32_t abi_version;
	void* host_ctx;
	cliproxy_host_call_fn call;
	cliproxy_host_free_fn free_buffer;
} cliproxy_host_api;

typedef int (*cliproxy_plugin_call_fn)(char*, uint8_t*, size_t, cliproxy_buffer*);
typedef void (*cliproxy_plugin_free_fn)(void*, size_t);
typedef void (*cliproxy_plugin_shutdown_fn)(void);

typedef struct {
	uint32_t abi_version;
	cliproxy_plugin_call_fn call;
	cliproxy_plugin_free_fn free_buffer;
	cliproxy_plugin_shutdown_fn shutdown;
} cliproxy_plugin_api;

extern int cliproxyPluginCall(char*, uint8_t*, size_t, cliproxy_buffer*);
extern void cliproxyPluginFree(void*, size_t);
extern void cliproxyPluginShutdown(void);

static const cliproxy_host_api* stored_host;

static void store_host_api(const cliproxy_host_api* host) {
	stored_host = host;
}

static int call_host_api(const char* method, const uint8_t* request, size_t request_len, cliproxy_buffer* response) {
	if (stored_host == NULL || stored_host->call == NULL) {
		return 1;
	}
	return stored_host->call(stored_host->host_ctx, method, request, request_len, response);
}

static void free_host_buffer(void* ptr, size_t len) {
	if (stored_host != NULL && stored_host->free_buffer != NULL && ptr != NULL) {
		stored_host->free_buffer(ptr, len);
	}
}
*/
import "C"

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
	"gopkg.in/yaml.v3"
)

const (
	providerID          = "github-copilot"
	defaultClientID     = "Iv1.b507a08c87ecfe98"
	defaultGitHubAPI    = "https://github.com"
	defaultGitHubREST   = "https://api.github.com"
	defaultCopilotAPI   = "https://api.githubcopilot.com"
	defaultEditor       = "vscode/1.104.0"
	defaultEditorPlugin = "copilot-chat/0.30.0"
	defaultUserAgent    = "GitHubCopilotChat/0.30.0"
	loginHTTPTimeout    = 20 * time.Second
	refreshSkew         = 5 * time.Minute
)

var (
	configMu      sync.RWMutex
	currentConfig = defaultConfig()
	loginStateMu  sync.Mutex
	loginStates   = map[string]loginState{}
)

type envelope struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *envelopeError  `json:"error,omitempty"`
}

type envelopeError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type lifecycleRequest struct {
	ConfigYAML []byte `json:"config_yaml"`
}

type registration struct {
	SchemaVersion uint32                 `json:"schema_version"`
	Metadata      pluginapi.Metadata     `json:"metadata"`
	Capabilities  registrationCapability `json:"capabilities"`
}

type registrationCapability struct {
	ModelProvider         bool                         `json:"model_provider"`
	AuthProvider          bool                         `json:"auth_provider"`
	Executor              bool                         `json:"executor"`
	ExecutorModelScope    pluginapi.ExecutorModelScope `json:"executor_model_scope"`
	ExecutorInputFormats  []string                     `json:"executor_input_formats,omitempty"`
	ExecutorOutputFormats []string                     `json:"executor_output_formats,omitempty"`
}

type identifierResponse struct {
	Identifier string `json:"identifier"`
}

type streamResponse struct {
	Headers http.Header                     `json:"headers,omitempty"`
	Chunks  []pluginapi.ExecutorStreamChunk `json:"chunks,omitempty"`
}

type rpcExecutorRequest struct {
	pluginapi.ExecutorRequest
	StreamID       string `json:"stream_id,omitempty"`
	HostCallbackID string `json:"host_callback_id,omitempty"`
}

type pluginConfig struct {
	ClientID          string   `yaml:"client-id"`
	GitHubBaseURL     string   `yaml:"github-base-url"`
	GitHubAPIBaseURL  string   `yaml:"github-api-base-url"`
	CopilotAPIBaseURL string   `yaml:"copilot-api-base-url"`
	Models            []string `yaml:"models"`
	EditorVersion     string   `yaml:"editor-version"`
	EditorPlugin      string   `yaml:"editor-plugin-version"`
	UserAgent         string   `yaml:"user-agent"`
	IntegrationID     string   `yaml:"integration-id"`
}

type deviceCodeResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

type accessTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Scope       string `json:"scope"`
	Error       string `json:"error"`
	ErrorURI    string `json:"error_uri"`
	Description string `json:"error_description"`
}

type loginState struct {
	DeviceCode string    `json:"device_code"`
	UserCode   string    `json:"user_code"`
	Interval   int       `json:"interval"`
	ExpiresAt  time.Time `json:"expires_at"`
	Config     pluginConfig
}

type copilotStorage struct {
	Type               string            `json:"type"`
	GitHubToken        string            `json:"github_token"`
	GitHubTokenType    string            `json:"github_token_type,omitempty"`
	Scope              string            `json:"scope,omitempty"`
	CopilotToken       string            `json:"copilot_token,omitempty"`
	CopilotTokenExpiry int64             `json:"copilot_token_expiry,omitempty"`
	CopilotEndpoints   map[string]string `json:"copilot_endpoints,omitempty"`
	LoginAt            time.Time         `json:"login_at,omitempty"`
}

func newLoginStateID() (string, error) {
	var raw [16]byte
	if _, errRead := rand.Read(raw[:]); errRead != nil {
		return "", fmt.Errorf("generate login state: %w", errRead)
	}
	return "ghc-" + hex.EncodeToString(raw[:]), nil
}

func storeLoginState(id string, state loginState) {
	loginStateMu.Lock()
	defer loginStateMu.Unlock()
	now := time.Now()
	for key, item := range loginStates {
		if now.After(item.ExpiresAt.Add(time.Minute)) {
			delete(loginStates, key)
		}
	}
	loginStates[id] = state
}

func loadLoginState(id string) (loginState, bool) {
	loginStateMu.Lock()
	defer loginStateMu.Unlock()
	state, ok := loginStates[id]
	return state, ok
}

func deleteLoginState(id string) {
	loginStateMu.Lock()
	delete(loginStates, id)
	loginStateMu.Unlock()
}

type copilotTokenResponse struct {
	Token         string            `json:"token"`
	ExpiresAt     int64             `json:"expires_at"`
	RefreshIn     int64             `json:"refresh_in"`
	Endpoints     map[string]string `json:"endpoints"`
	Error         string            `json:"error"`
	Message       string            `json:"message"`
	Documentation string            `json:"documentation_url"`
}

type modelsResponse struct {
	Data   []modelEntry `json:"data"`
	Models []modelEntry `json:"models"`
}

type modelEntry struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Object      string `json:"object"`
	DisplayName string `json:"display_name"`
}

type hostHTTPRequest struct {
	HostCallbackID string      `json:"host_callback_id,omitempty"`
	Method         string      `json:"method,omitempty"`
	URL            string      `json:"url,omitempty"`
	Headers        http.Header `json:"headers,omitempty"`
	Body           []byte      `json:"body,omitempty"`
}

type hostHTTPResponse struct {
	StatusCode int         `json:"status_code"`
	Headers    http.Header `json:"headers,omitempty"`
	Body       []byte      `json:"body,omitempty"`
}

func (r *hostHTTPResponse) UnmarshalJSON(raw []byte) error {
	type snake hostHTTPResponse
	var in struct {
		snake
		StatusCodeCamel int         `json:"StatusCode"`
		HeadersCamel    http.Header `json:"Headers"`
		BodyCamel       []byte      `json:"Body"`
	}
	if errDecode := json.Unmarshal(raw, &in); errDecode != nil {
		return errDecode
	}
	*r = hostHTTPResponse(in.snake)
	if r.StatusCode == 0 {
		r.StatusCode = in.StatusCodeCamel
	}
	if len(r.Headers) == 0 {
		r.Headers = in.HeadersCamel
	}
	if len(r.Body) == 0 {
		r.Body = in.BodyCamel
	}
	return nil
}

type hostHTTPStreamResponse struct {
	StatusCode int         `json:"status_code"`
	Headers    http.Header `json:"headers,omitempty"`
	StreamID   string      `json:"stream_id,omitempty"`
}

func (r *hostHTTPStreamResponse) UnmarshalJSON(raw []byte) error {
	type snake hostHTTPStreamResponse
	var in struct {
		snake
		StatusCodeCamel int         `json:"StatusCode"`
		HeadersCamel    http.Header `json:"Headers"`
		StreamIDCamel   string      `json:"StreamID"`
	}
	if errDecode := json.Unmarshal(raw, &in); errDecode != nil {
		return errDecode
	}
	*r = hostHTTPStreamResponse(in.snake)
	if r.StatusCode == 0 {
		r.StatusCode = in.StatusCodeCamel
	}
	if len(r.Headers) == 0 {
		r.Headers = in.HeadersCamel
	}
	if r.StreamID == "" {
		r.StreamID = in.StreamIDCamel
	}
	return nil
}

type hostHTTPStreamReadRequest struct {
	StreamID string `json:"stream_id"`
}

type hostHTTPStreamReadResponse struct {
	Payload []byte `json:"payload,omitempty"`
	Error   string `json:"error,omitempty"`
	Done    bool   `json:"done,omitempty"`
}

type hostHTTPStreamCloseRequest struct {
	StreamID string `json:"stream_id"`
}

type pluginStreamEmitRequest struct {
	StreamID string `json:"stream_id"`
	Payload  []byte `json:"payload,omitempty"`
	Error    string `json:"error,omitempty"`
}

type pluginStreamCloseRequest struct {
	StreamID string `json:"stream_id"`
	Error    string `json:"error,omitempty"`
}

func main() {}

//export cliproxy_plugin_init
func cliproxy_plugin_init(host *C.cliproxy_host_api, plugin *C.cliproxy_plugin_api) C.int {
	if plugin == nil {
		return 1
	}
	C.store_host_api(host)
	plugin.abi_version = C.uint32_t(pluginabi.ABIVersion)
	plugin.call = C.cliproxy_plugin_call_fn(C.cliproxyPluginCall)
	plugin.free_buffer = C.cliproxy_plugin_free_fn(C.cliproxyPluginFree)
	plugin.shutdown = C.cliproxy_plugin_shutdown_fn(C.cliproxyPluginShutdown)
	return 0
}

//export cliproxyPluginCall
func cliproxyPluginCall(method *C.char, request *C.uint8_t, requestLen C.size_t, response *C.cliproxy_buffer) C.int {
	if response != nil {
		response.ptr = nil
		response.len = 0
	}
	if method == nil {
		writeResponse(response, errorEnvelope("invalid_method", "method is required"))
		return 1
	}
	var requestBytes []byte
	if request != nil && requestLen > 0 {
		requestBytes = C.GoBytes(unsafe.Pointer(request), C.int(requestLen))
	}
	raw, errHandle := handleMethod(C.GoString(method), requestBytes)
	if errHandle != nil {
		writeResponse(response, errorEnvelope("plugin_error", errHandle.Error()))
		return 1
	}
	writeResponse(response, raw)
	return 0
}

//export cliproxyPluginFree
func cliproxyPluginFree(ptr unsafe.Pointer, len C.size_t) {
	if ptr != nil {
		C.free(ptr)
	}
	_ = len
}

//export cliproxyPluginShutdown
func cliproxyPluginShutdown() {
	loginStateMu.Lock()
	loginStates = map[string]loginState{}
	loginStateMu.Unlock()
}

func handleMethod(method string, request []byte) ([]byte, error) {
	switch method {
	case pluginabi.MethodPluginRegister, pluginabi.MethodPluginReconfigure:
		cfg, errConfig := configFromLifecycle(request)
		if errConfig != nil {
			return nil, errConfig
		}
		setConfig(cfg)
		return okEnvelope(pluginRegistration(cfg))
	case pluginabi.MethodAuthIdentifier, pluginabi.MethodExecutorIdentifier:
		return okEnvelope(identifierResponse{Identifier: providerID})
	case pluginabi.MethodAuthLoginStart:
		return authLoginStart(request)
	case pluginabi.MethodAuthLoginPoll:
		return authLoginPoll(request)
	case pluginabi.MethodAuthParse:
		return authParse(request)
	case pluginabi.MethodAuthRefresh:
		return authRefresh(request)
	case pluginabi.MethodModelStatic:
		return okEnvelope(modelResponse(getConfig().Models, nil))
	case pluginabi.MethodModelForAuth:
		return modelsForAuth(request)
	case pluginabi.MethodExecutorExecute:
		return execute(request, false)
	case pluginabi.MethodExecutorExecuteStream:
		return executeStream(request)
	case pluginabi.MethodExecutorCountTokens:
		return okEnvelope(pluginapi.ExecutorResponse{Payload: []byte(`{"total_tokens":0}`)})
	case pluginabi.MethodExecutorHTTPRequest:
		return executorHTTPRequest(request)
	default:
		return errorEnvelope("unknown_method", "unknown method: "+method), nil
	}
}

func pluginRegistration(cfg pluginConfig) registration {
	_ = cfg
	return registration{
		SchemaVersion: pluginabi.SchemaVersion,
		Metadata: pluginapi.Metadata{
			Name:             "GitHub Copilot",
			Version:          "0.1.0",
			Author:           "router-for-me",
			GitHubRepository: "https://github.com/router-for-me/CLIProxyAPI",
			ConfigFields: []pluginapi.ConfigField{
				{Name: "client-id", Type: pluginapi.ConfigFieldTypeString, Description: "GitHub OAuth device-flow client id."},
				{Name: "github-base-url", Type: pluginapi.ConfigFieldTypeString, Description: "GitHub web OAuth base URL."},
				{Name: "github-api-base-url", Type: pluginapi.ConfigFieldTypeString, Description: "GitHub REST API base URL."},
				{Name: "copilot-api-base-url", Type: pluginapi.ConfigFieldTypeString, Description: "GitHub Copilot API base URL."},
				{Name: "models", Type: pluginapi.ConfigFieldTypeArray, Description: "Fallback model ids when Copilot model discovery is unavailable."},
				{Name: "editor-version", Type: pluginapi.ConfigFieldTypeString, Description: "Editor-Version header sent to Copilot."},
				{Name: "editor-plugin-version", Type: pluginapi.ConfigFieldTypeString, Description: "Editor-Plugin-Version header sent to Copilot."},
				{Name: "user-agent", Type: pluginapi.ConfigFieldTypeString, Description: "User-Agent header sent to GitHub and Copilot."},
				{Name: "integration-id", Type: pluginapi.ConfigFieldTypeString, Description: "Optional Copilot-Integration-Id header."},
			},
		},
		Capabilities: registrationCapability{
			ModelProvider:         true,
			AuthProvider:          true,
			Executor:              true,
			ExecutorModelScope:    pluginapi.ExecutorModelScopeOAuth,
			ExecutorInputFormats:  []string{"chat-completions"},
			ExecutorOutputFormats: []string{"chat-completions"},
		},
	}
}

func authLoginStart(raw []byte) ([]byte, error) {
	_ = raw
	cfg := getConfig()
	values := url.Values{}
	values.Set("client_id", cfg.ClientID)
	values.Set("scope", "read:user")
	resp, errDo := hostDoWithTimeout(http.MethodPost, trimRightSlash(cfg.GitHubBaseURL)+"/login/device/code", formHeaders(cfg), []byte(values.Encode()), loginHTTPTimeout)
	if errDo != nil {
		return nil, errDo
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("github device-code request failed with status %d: %s", resp.StatusCode, string(resp.Body))
	}
	var decoded deviceCodeResponse
	if errDecode := json.Unmarshal(resp.Body, &decoded); errDecode != nil {
		return nil, fmt.Errorf("decode github device-code response: %w", errDecode)
	}
	if strings.TrimSpace(decoded.DeviceCode) == "" {
		return nil, fmt.Errorf("github device-code response did not include device_code")
	}
	expires := time.Now().Add(time.Duration(decoded.ExpiresIn) * time.Second).UTC()
	if decoded.ExpiresIn <= 0 {
		expires = time.Now().Add(15 * time.Minute).UTC()
	}
	stateID, errState := newLoginStateID()
	if errState != nil {
		return nil, errState
	}
	storeLoginState(stateID, loginState{
		DeviceCode: decoded.DeviceCode,
		UserCode:   decoded.UserCode,
		Interval:   decoded.Interval,
		ExpiresAt:  expires,
		Config:     cfg,
	})
	loginURL := decoded.VerificationURIComplete
	if strings.TrimSpace(loginURL) == "" {
		loginURL = decoded.VerificationURI
	}
	return okEnvelope(pluginapi.AuthLoginStartResponse{
		Provider:  providerID,
		URL:       loginURL,
		State:     stateID,
		ExpiresAt: expires,
		Metadata: map[string]any{
			"user_code": decoded.UserCode,
			"interval":  decoded.Interval,
			"flow":      "device",
		},
	})
}

func authLoginPoll(raw []byte) ([]byte, error) {
	var req pluginapi.AuthLoginPollRequest
	if errDecode := json.Unmarshal(raw, &req); errDecode != nil {
		return nil, errDecode
	}
	state, okState := loadLoginState(req.State)
	if !okState {
		return okEnvelope(pluginapi.AuthLoginPollResponse{Status: pluginapi.AuthLoginStatusError, Message: "GitHub Copilot login state expired or not found"})
	}
	if time.Now().After(state.ExpiresAt) {
		deleteLoginState(req.State)
		return okEnvelope(pluginapi.AuthLoginPollResponse{Status: pluginapi.AuthLoginStatusError, Message: "GitHub device login expired"})
	}
	cfg := normalizeConfig(state.Config)
	values := url.Values{}
	values.Set("client_id", cfg.ClientID)
	values.Set("device_code", state.DeviceCode)
	values.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
	resp, errDo := hostDoWithTimeout(http.MethodPost, trimRightSlash(cfg.GitHubBaseURL)+"/login/oauth/access_token", formHeaders(cfg), []byte(values.Encode()), loginHTTPTimeout)
	if errDo != nil {
		return nil, errDo
	}
	var decoded accessTokenResponse
	if errDecode := json.Unmarshal(resp.Body, &decoded); errDecode != nil {
		return nil, fmt.Errorf("decode github token response: %w", errDecode)
	}
	switch strings.TrimSpace(decoded.Error) {
	case "":
	case "authorization_pending":
		return okEnvelope(pluginapi.AuthLoginPollResponse{Status: pluginapi.AuthLoginStatusPending, Message: "Waiting for GitHub authorization"})
	case "slow_down":
		return okEnvelope(pluginapi.AuthLoginPollResponse{Status: pluginapi.AuthLoginStatusPending, Message: "GitHub asked to slow down polling"})
	default:
		msg := decoded.Description
		if msg == "" {
			msg = decoded.Error
		}
		return okEnvelope(pluginapi.AuthLoginPollResponse{Status: pluginapi.AuthLoginStatusError, Message: msg})
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("github token request failed with status %d: %s", resp.StatusCode, string(resp.Body))
	}
	if strings.TrimSpace(decoded.AccessToken) == "" {
		return okEnvelope(pluginapi.AuthLoginPollResponse{Status: pluginapi.AuthLoginStatusPending, Message: "Waiting for GitHub authorization"})
	}
	deleteLoginState(req.State)
	storage := copilotStorage{
		Type:            providerID,
		GitHubToken:     decoded.AccessToken,
		GitHubTokenType: decoded.TokenType,
		Scope:           decoded.Scope,
		LoginAt:         time.Now().UTC(),
	}
	auth, errAuth := authDataFromStorage(storage, "GitHub Copilot")
	if errAuth != nil {
		return nil, errAuth
	}
	return okEnvelope(pluginapi.AuthLoginPollResponse{
		Status:  pluginapi.AuthLoginStatusSuccess,
		Message: "GitHub Copilot login complete",
		Auth:    auth,
	})
}

func authParse(raw []byte) ([]byte, error) {
	var req pluginapi.AuthParseRequest
	if errDecode := json.Unmarshal(raw, &req); errDecode != nil {
		return nil, errDecode
	}
	var storage copilotStorage
	if errDecode := json.Unmarshal(req.RawJSON, &storage); errDecode != nil || storage.Type != providerID || strings.TrimSpace(storage.GitHubToken) == "" {
		return okEnvelope(pluginapi.AuthParseResponse{Handled: false})
	}
	auth, errAuth := authDataFromStorage(storage, authLabel(req.FileName))
	if errAuth != nil {
		return nil, errAuth
	}
	return okEnvelope(pluginapi.AuthParseResponse{Handled: true, Auth: auth})
}

func authRefresh(raw []byte) ([]byte, error) {
	var req pluginapi.AuthRefreshRequest
	if errDecode := json.Unmarshal(raw, &req); errDecode != nil {
		return nil, errDecode
	}
	storage, errStorage := storageFromRaw(req.StorageJSON)
	if errStorage != nil {
		return nil, errStorage
	}
	updated, errToken := ensureCopilotToken(storage)
	if errToken != nil {
		return nil, errToken
	}
	auth, errAuth := authDataFromStorage(updated, authLabel(req.AuthID))
	if errAuth != nil {
		return nil, errAuth
	}
	return okEnvelope(pluginapi.AuthRefreshResponse{
		Auth:             auth,
		NextRefreshAfter: nextRefreshAfter(updated),
	})
}

func modelsForAuth(raw []byte) ([]byte, error) {
	var req pluginapi.AuthModelRequest
	if errDecode := json.Unmarshal(raw, &req); errDecode != nil {
		return nil, errDecode
	}
	storage, errStorage := storageFromRaw(req.StorageJSON)
	if errStorage != nil {
		return okEnvelope(modelResponse(getConfig().Models, nil))
	}
	storage, errToken := ensureCopilotToken(storage)
	if errToken != nil {
		return okEnvelope(modelResponse(getConfig().Models, nil))
	}
	models := fetchModels(storage)
	authUpdate, _ := authDataFromStorage(storage, authLabel(req.AuthID))
	return okEnvelope(modelResponse(models, &authUpdate))
}

func execute(raw []byte, stream bool) ([]byte, error) {
	var req pluginapi.ExecutorRequest
	if errDecode := json.Unmarshal(raw, &req); errDecode != nil {
		return nil, errDecode
	}
	storage, errStorage := storageFromRaw(req.StorageJSON)
	if errStorage != nil {
		return nil, errStorage
	}
	storage, errToken := ensureCopilotToken(storage)
	if errToken != nil {
		return nil, errToken
	}
	body := rewriteModel(req.Payload, req.Model, stream)
	resp, errDo := hostDo(http.MethodPost, copilotChatCompletionsURL(storage), copilotHeaders(storage), body)
	if errDo != nil {
		return nil, errDo
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("github copilot chat request failed with status %d: %s", resp.StatusCode, string(resp.Body))
	}
	return okEnvelope(pluginapi.ExecutorResponse{Payload: resp.Body, Headers: resp.Headers})
}

func executeStream(raw []byte) ([]byte, error) {
	var req rpcExecutorRequest
	if errDecode := json.Unmarshal(raw, &req); errDecode != nil {
		return nil, errDecode
	}
	if strings.TrimSpace(req.StreamID) == "" {
		return errorEnvelope("executor_error", "stream_id is required for executor.execute_stream"), nil
	}
	go forwardCopilotStream(req)
	return okEnvelope(streamResponse{Headers: http.Header{"Content-Type": []string{"text/event-stream"}}})
}

func executorHTTPRequest(raw []byte) ([]byte, error) {
	var req pluginapi.ExecutorHTTPRequest
	if errDecode := json.Unmarshal(raw, &req); errDecode != nil {
		return nil, errDecode
	}
	storage, errStorage := storageFromRaw(req.StorageJSON)
	if errStorage != nil {
		return nil, errStorage
	}
	storage, errToken := ensureCopilotToken(storage)
	if errToken != nil {
		return nil, errToken
	}
	resp, errDo := hostDo(req.Method, req.URL, mergeHeaders(req.Headers, copilotHeaders(storage)), req.Body)
	if errDo != nil {
		return nil, errDo
	}
	return okEnvelope(pluginapi.ExecutorHTTPResponse{StatusCode: resp.StatusCode, Headers: resp.Headers, Body: resp.Body})
}

func ensureCopilotToken(storage copilotStorage) (copilotStorage, error) {
	if strings.TrimSpace(storage.GitHubToken) == "" {
		return storage, fmt.Errorf("github copilot auth is missing github_token")
	}
	if strings.TrimSpace(storage.CopilotToken) != "" && time.Unix(storage.CopilotTokenExpiry, 0).After(time.Now().Add(refreshSkew)) {
		return storage, nil
	}
	cfg := getConfig()
	headers := jsonHeaders(cfg)
	headers.Set("Authorization", "token "+storage.GitHubToken)
	resp, errDo := hostDo(http.MethodGet, trimRightSlash(cfg.GitHubAPIBaseURL)+"/copilot_internal/v2/token", headers, nil)
	if errDo != nil {
		return storage, errDo
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return storage, fmt.Errorf("github copilot token request failed with status %d: %s", resp.StatusCode, string(resp.Body))
	}
	var decoded copilotTokenResponse
	if errDecode := json.Unmarshal(resp.Body, &decoded); errDecode != nil {
		return storage, fmt.Errorf("decode github copilot token response: %w", errDecode)
	}
	if strings.TrimSpace(decoded.Error) != "" {
		msg := decoded.Message
		if msg == "" {
			msg = decoded.Error
		}
		return storage, fmt.Errorf("github copilot token error: %s", msg)
	}
	if strings.TrimSpace(decoded.Token) == "" {
		return storage, fmt.Errorf("github copilot token response did not include token")
	}
	storage.CopilotToken = decoded.Token
	storage.CopilotTokenExpiry = decoded.ExpiresAt
	storage.CopilotEndpoints = decoded.Endpoints
	return storage, nil
}

func fetchModels(storage copilotStorage) []string {
	headers := copilotHeaders(storage)
	resp, errDo := hostDo(http.MethodGet, copilotBaseURL(storage)+"/models", headers, nil)
	if errDo != nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return getConfig().Models
	}
	var decoded modelsResponse
	if errDecode := json.Unmarshal(resp.Body, &decoded); errDecode != nil {
		return getConfig().Models
	}
	entries := decoded.Data
	if len(entries) == 0 {
		entries = decoded.Models
	}
	models := make([]string, 0, len(entries))
	seen := make(map[string]struct{})
	for _, entry := range entries {
		id := strings.TrimSpace(entry.ID)
		if id == "" {
			id = strings.TrimSpace(entry.Name)
		}
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		models = append(models, id)
	}
	if len(models) == 0 {
		return getConfig().Models
	}
	return models
}

func modelResponse(models []string, authUpdate *pluginapi.AuthData) pluginapi.ModelResponse {
	out := make([]pluginapi.ModelInfo, 0, len(models))
	for _, model := range normalizedModels(models) {
		out = append(out, pluginapi.ModelInfo{
			ID:                         model,
			Object:                     "model",
			OwnedBy:                    providerID,
			DisplayName:                model,
			SupportedGenerationMethods: []string{"chat"},
			UserDefined:                true,
		})
	}
	resp := pluginapi.ModelResponse{Provider: providerID, Models: out}
	if authUpdate != nil {
		resp.AuthUpdate = *authUpdate
	}
	return resp
}

func rewriteModel(raw []byte, model string, stream bool) []byte {
	body := map[string]any{}
	if errDecode := json.Unmarshal(raw, &body); errDecode != nil {
		return append([]byte(nil), raw...)
	}
	cleanModel := strings.TrimSpace(model)
	if cleanModel != "" {
		body["model"] = cleanModel
	}
	body["stream"] = stream
	sanitizeCopilotChatPayload(body)
	out, errMarshal := json.Marshal(body)
	if errMarshal != nil {
		return append([]byte(nil), raw...)
	}
	return out
}

func sanitizeCopilotChatPayload(body map[string]any) {
	if body == nil {
		return
	}
	if _, ok := body["max_tokens"]; !ok {
		if value, exists := body["max_completion_tokens"]; exists {
			body["max_tokens"] = value
		}
	}
	for _, key := range []string{
		"max_completion_tokens",
		"metadata",
		"reasoning",
		"reasoning_effort",
		"reasoning_summary",
		"service_tier",
		"store",
		"stream_options",
		"verbosity",
	} {
		delete(body, key)
	}
}

func authDataFromStorage(storage copilotStorage, label string) (pluginapi.AuthData, error) {
	storage.Type = providerID
	raw, errMarshal := json.Marshal(storage)
	if errMarshal != nil {
		return pluginapi.AuthData{}, errMarshal
	}
	id := "github-copilot-" + tokenFingerprint(storage.GitHubToken)
	return pluginapi.AuthData{
		Provider:         providerID,
		ID:               id,
		FileName:         id + ".json",
		Label:            label,
		StorageJSON:      raw,
		Metadata:         map[string]any{"type": providerID},
		NextRefreshAfter: nextRefreshAfter(storage),
	}, nil
}

func tokenFingerprint(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])[:12]
}

func storageFromRaw(raw []byte) (copilotStorage, error) {
	var storage copilotStorage
	if errDecode := json.Unmarshal(raw, &storage); errDecode != nil {
		return copilotStorage{}, fmt.Errorf("decode github copilot auth storage: %w", errDecode)
	}
	if storage.Type != "" && storage.Type != providerID {
		return copilotStorage{}, fmt.Errorf("unexpected github copilot auth storage type %q", storage.Type)
	}
	return storage, nil
}

func nextRefreshAfter(storage copilotStorage) time.Time {
	expiry := time.Unix(storage.CopilotTokenExpiry, 0)
	if strings.TrimSpace(storage.CopilotToken) == "" || storage.CopilotTokenExpiry <= 0 {
		return time.Now().Add(time.Minute).UTC()
	}
	next := expiry.Add(-refreshSkew)
	if next.Before(time.Now().Add(time.Minute)) {
		return time.Now().Add(time.Minute).UTC()
	}
	return next.UTC()
}

func configFromLifecycle(raw []byte) (pluginConfig, error) {
	cfg := defaultConfig()
	if len(raw) == 0 {
		return cfg, nil
	}
	var req lifecycleRequest
	if errDecode := json.Unmarshal(raw, &req); errDecode != nil {
		return cfg, errDecode
	}
	if len(req.ConfigYAML) == 0 {
		return cfg, nil
	}
	var parsed pluginConfig
	if errYAML := yaml.Unmarshal(req.ConfigYAML, &parsed); errYAML != nil {
		return cfg, fmt.Errorf("parse github copilot plugin config: %w", errYAML)
	}
	return normalizeConfig(mergeConfig(cfg, parsed)), nil
}

func defaultConfig() pluginConfig {
	return pluginConfig{
		ClientID:          defaultClientID,
		GitHubBaseURL:     defaultGitHubAPI,
		GitHubAPIBaseURL:  defaultGitHubREST,
		CopilotAPIBaseURL: defaultCopilotAPI,
		Models:            []string{"gpt-4.1", "gpt-4o", "claude-sonnet-4"},
		EditorVersion:     defaultEditor,
		EditorPlugin:      defaultEditorPlugin,
		UserAgent:         defaultUserAgent,
	}
}

func mergeConfig(base pluginConfig, override pluginConfig) pluginConfig {
	if override.ClientID != "" {
		base.ClientID = override.ClientID
	}
	if override.GitHubBaseURL != "" {
		base.GitHubBaseURL = override.GitHubBaseURL
	}
	if override.GitHubAPIBaseURL != "" {
		base.GitHubAPIBaseURL = override.GitHubAPIBaseURL
	}
	if override.CopilotAPIBaseURL != "" {
		base.CopilotAPIBaseURL = override.CopilotAPIBaseURL
	}
	if len(override.Models) > 0 {
		base.Models = override.Models
	}
	if override.EditorVersion != "" {
		base.EditorVersion = override.EditorVersion
	}
	if override.EditorPlugin != "" {
		base.EditorPlugin = override.EditorPlugin
	}
	if override.UserAgent != "" {
		base.UserAgent = override.UserAgent
	}
	if override.IntegrationID != "" {
		base.IntegrationID = override.IntegrationID
	}
	return base
}

func normalizeConfig(cfg pluginConfig) pluginConfig {
	if strings.TrimSpace(cfg.ClientID) == "" {
		cfg.ClientID = defaultClientID
	}
	if strings.TrimSpace(cfg.GitHubBaseURL) == "" {
		cfg.GitHubBaseURL = defaultGitHubAPI
	}
	if strings.TrimSpace(cfg.GitHubAPIBaseURL) == "" {
		cfg.GitHubAPIBaseURL = defaultGitHubREST
	}
	if strings.TrimSpace(cfg.CopilotAPIBaseURL) == "" {
		cfg.CopilotAPIBaseURL = defaultCopilotAPI
	}
	if strings.TrimSpace(cfg.EditorVersion) == "" {
		cfg.EditorVersion = defaultEditor
	}
	if strings.TrimSpace(cfg.EditorPlugin) == "" {
		cfg.EditorPlugin = defaultEditorPlugin
	}
	if strings.TrimSpace(cfg.UserAgent) == "" {
		cfg.UserAgent = defaultUserAgent
	}
	cfg.Models = normalizedModels(cfg.Models)
	return cfg
}

func normalizedModels(models []string) []string {
	if len(models) == 0 {
		models = defaultConfig().Models
	}
	out := make([]string, 0, len(models))
	seen := make(map[string]struct{})
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		if _, ok := seen[model]; ok {
			continue
		}
		seen[model] = struct{}{}
		out = append(out, model)
	}
	if len(out) == 0 {
		return defaultConfig().Models
	}
	return out
}

func setConfig(cfg pluginConfig) {
	configMu.Lock()
	currentConfig = normalizeConfig(cfg)
	configMu.Unlock()
}

func getConfig() pluginConfig {
	configMu.RLock()
	defer configMu.RUnlock()
	return currentConfig
}

func formHeaders(cfg pluginConfig) http.Header {
	headers := jsonHeaders(cfg)
	headers.Set("Content-Type", "application/x-www-form-urlencoded")
	return headers
}

func jsonHeaders(cfg pluginConfig) http.Header {
	headers := http.Header{}
	headers.Set("Accept", "application/json")
	headers.Set("User-Agent", cfg.UserAgent)
	return headers
}

func copilotHeaders(storage copilotStorage) http.Header {
	cfg := getConfig()
	headers := jsonHeaders(cfg)
	headers.Set("Authorization", "Bearer "+storage.CopilotToken)
	headers.Set("Content-Type", "application/json")
	headers.Set("Editor-Version", cfg.EditorVersion)
	headers.Set("Editor-Plugin-Version", cfg.EditorPlugin)
	if strings.TrimSpace(cfg.IntegrationID) != "" {
		headers.Set("Copilot-Integration-Id", cfg.IntegrationID)
	}
	return headers
}

func mergeHeaders(base http.Header, overlay http.Header) http.Header {
	out := http.Header{}
	for key, values := range base {
		out[key] = append([]string(nil), values...)
	}
	for key, values := range overlay {
		out[key] = append([]string(nil), values...)
	}
	return out
}

func copilotBaseURL(storage copilotStorage) string {
	if storage.CopilotEndpoints != nil {
		if api := strings.TrimSpace(storage.CopilotEndpoints["api"]); api != "" {
			return trimRightSlash(api)
		}
		if proxy := strings.TrimSpace(storage.CopilotEndpoints["proxy"]); proxy != "" {
			return trimRightSlash(proxy)
		}
	}
	return trimRightSlash(getConfig().CopilotAPIBaseURL)
}

func copilotChatCompletionsURL(storage copilotStorage) string {
	return copilotBaseURL(storage) + "/chat/completions"
}

func trimRightSlash(value string) string {
	return strings.TrimRight(strings.TrimSpace(value), "/")
}

func authLabel(fallback string) string {
	fallback = strings.TrimSpace(fallback)
	if fallback == "" {
		return "GitHub Copilot"
	}
	return fallback
}

func forwardCopilotStream(req rpcExecutorRequest) {
	streamID := strings.TrimSpace(req.StreamID)
	defer closePluginStream(streamID, "")
	storage, errStorage := storageFromRaw(req.StorageJSON)
	if errStorage != nil {
		closePluginStream(streamID, errStorage.Error())
		return
	}
	storage, errToken := ensureCopilotToken(storage)
	if errToken != nil {
		closePluginStream(streamID, errToken.Error())
		return
	}
	body := rewriteModel(req.Payload, req.Model, true)
	stream, errDo := hostDoStream(req.HostCallbackID, http.MethodPost, copilotChatCompletionsURL(storage), copilotHeaders(storage), body)
	if errDo != nil {
		closePluginStream(streamID, errDo.Error())
		return
	}
	if stream.StatusCode < 200 || stream.StatusCode >= 300 {
		body := readCopilotStreamErrorBody(stream.StreamID, 4096)
		closeHostStream(stream.StreamID)
		msg := fmt.Sprintf("github copilot stream request failed with status %d", stream.StatusCode)
		if body != "" {
			msg += ": " + body
		}
		closePluginStream(streamID, msg)
		return
	}
	defer closeHostStream(stream.StreamID)
	framer := &copilotStreamFramer{}
	for {
		chunk, errRead := readHostStream(stream.StreamID)
		if errRead != nil {
			closePluginStream(streamID, errRead.Error())
			return
		}
		if len(chunk.Payload) > 0 {
			if errEmit := emitCopilotStreamPayload(streamID, framer.Feed(chunk.Payload)); errEmit != nil {
				closePluginStream(streamID, errEmit.Error())
				return
			}
		}
		if chunk.Error != "" {
			closePluginStream(streamID, chunk.Error)
			return
		}
		if chunk.Done {
			if errEmit := emitCopilotStreamPayload(streamID, framer.Flush()); errEmit != nil {
				closePluginStream(streamID, errEmit.Error())
				return
			}
			return
		}
	}
}

func readCopilotStreamErrorBody(streamID string, limit int) string {
	if strings.TrimSpace(streamID) == "" || limit <= 0 {
		return ""
	}
	var buf bytes.Buffer
	for buf.Len() < limit {
		chunk, errRead := readHostStream(streamID)
		if errRead != nil {
			break
		}
		if len(chunk.Payload) > 0 {
			remaining := limit - buf.Len()
			if len(chunk.Payload) > remaining {
				buf.Write(chunk.Payload[:remaining])
			} else {
				buf.Write(chunk.Payload)
			}
		}
		if chunk.Done || chunk.Error != "" {
			break
		}
	}
	return strings.TrimSpace(buf.String())
}

func emitCopilotStreamPayload(streamID string, frames [][]byte) error {
	for _, frame := range frames {
		if len(frame) == 0 {
			continue
		}
		if errEmit := emitPluginStream(streamID, frame); errEmit != nil {
			return errEmit
		}
	}
	return nil
}

type copilotStreamFramer struct {
	buffer []byte
	sawSSE bool
}

func (f *copilotStreamFramer) Feed(payload []byte) [][]byte {
	if f == nil || len(payload) == 0 {
		return nil
	}
	f.buffer = append(f.buffer, payload...)
	frames := make([][]byte, 0)
	for {
		event, rest, ok := popCopilotSSEEvent(f.buffer)
		if !ok {
			break
		}
		f.buffer = rest
		if frame, okFrame := copilotSSEEventFrame(event); okFrame {
			f.sawSSE = true
			frames = append(frames, frame)
		}
	}
	if len(frames) > 0 || f.sawSSE || bytes.Contains(f.buffer, []byte("data:")) {
		return frames
	}
	trimmed := bytes.TrimSpace(f.buffer)
	if len(trimmed) == 0 {
		return nil
	}
	f.buffer = nil
	return [][]byte{bytes.Clone(trimmed)}
}

func (f *copilotStreamFramer) Flush() [][]byte {
	if f == nil {
		return nil
	}
	if len(bytes.TrimSpace(f.buffer)) == 0 {
		f.buffer = nil
		return nil
	}
	if frame, okFrame := copilotSSEEventFrame(f.buffer); okFrame {
		f.buffer = nil
		f.sawSSE = true
		return [][]byte{frame}
	}
	if f.sawSSE {
		f.buffer = nil
		return nil
	}
	trimmed := bytes.TrimSpace(f.buffer)
	f.buffer = nil
	return [][]byte{bytes.Clone(trimmed)}
}

func copilotStreamFrames(payload []byte) [][]byte {
	framer := &copilotStreamFramer{}
	frames := framer.Feed(payload)
	return append(frames, framer.Flush()...)
}

func popCopilotSSEEvent(payload []byte) ([]byte, []byte, bool) {
	for i := 0; i < len(payload); i++ {
		if payload[i] != '\n' {
			continue
		}
		lineStart := i
		for lineStart > 0 && payload[lineStart-1] != '\n' {
			lineStart--
		}
		line := bytes.TrimRight(payload[lineStart:i], "\r")
		if len(bytes.TrimSpace(line)) != 0 {
			continue
		}
		event := bytes.TrimSpace(payload[:lineStart])
		rest := payload[i+1:]
		if len(event) == 0 {
			payload = rest
			i = -1
			continue
		}
		return event, rest, true
	}
	return nil, payload, false
}

func copilotSSEEventFrame(event []byte) ([]byte, bool) {
	eventData := make([][]byte, 0)
	for _, rawLine := range bytes.Split(event, []byte("\n")) {
		line := bytes.TrimRight(rawLine, "\r")
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		data := bytes.TrimSpace(line[len("data:"):])
		if len(data) > 0 {
			eventData = append(eventData, bytes.Clone(data))
		}
	}
	if len(eventData) == 0 {
		return nil, false
	}
	joined := bytes.TrimSpace(bytes.Join(eventData, []byte("\n")))
	if len(joined) == 0 {
		return nil, false
	}
	return bytes.Clone(joined), true
}

func hostDo(method string, target string, headers http.Header, body []byte) (hostHTTPResponse, error) {
	result, errCall := callHost(pluginabi.MethodHostHTTPDo, hostHTTPRequest{
		Method:  method,
		URL:     target,
		Headers: headers,
		Body:    body,
	})
	if errCall != nil {
		return hostHTTPResponse{}, errCall
	}
	var resp hostHTTPResponse
	if errDecode := json.Unmarshal(result, &resp); errDecode != nil {
		return hostHTTPResponse{}, fmt.Errorf("decode host http response: %w", errDecode)
	}
	return resp, nil
}

func hostDoWithTimeout(method string, target string, headers http.Header, body []byte, timeout time.Duration) (hostHTTPResponse, error) {
	if timeout <= 0 {
		return hostDo(method, target, headers, body)
	}
	type result struct {
		resp hostHTTPResponse
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		resp, errDo := hostDo(method, target, headers, body)
		ch <- result{resp: resp, err: errDo}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	select {
	case item := <-ch:
		return item.resp, item.err
	case <-ctx.Done():
		return hostHTTPResponse{}, fmt.Errorf("credential acquisition timed out after %s", timeout)
	}
}

func hostDoStream(hostCallbackID string, method string, target string, headers http.Header, body []byte) (hostHTTPStreamResponse, error) {
	result, errCall := callHost(pluginabi.MethodHostHTTPDoStream, hostHTTPRequest{
		HostCallbackID: hostCallbackID,
		Method:         method,
		URL:            target,
		Headers:        headers,
		Body:           body,
	})
	if errCall != nil {
		return hostHTTPStreamResponse{}, errCall
	}
	var resp hostHTTPStreamResponse
	if errDecode := json.Unmarshal(result, &resp); errDecode != nil {
		return hostHTTPStreamResponse{}, fmt.Errorf("decode host http stream response: %w", errDecode)
	}
	return resp, nil
}

func readHostStream(streamID string) (hostHTTPStreamReadResponse, error) {
	result, errCall := callHost(pluginabi.MethodHostHTTPStreamRead, hostHTTPStreamReadRequest{StreamID: streamID})
	if errCall != nil {
		return hostHTTPStreamReadResponse{}, errCall
	}
	var resp hostHTTPStreamReadResponse
	if errDecode := json.Unmarshal(result, &resp); errDecode != nil {
		return hostHTTPStreamReadResponse{}, fmt.Errorf("decode host http stream chunk: %w", errDecode)
	}
	return resp, nil
}

func closeHostStream(streamID string) {
	if strings.TrimSpace(streamID) == "" {
		return
	}
	_, _ = callHost(pluginabi.MethodHostHTTPStreamClose, hostHTTPStreamCloseRequest{StreamID: streamID})
}

func emitPluginStream(streamID string, payload []byte) error {
	if strings.TrimSpace(streamID) == "" {
		return fmt.Errorf("plugin stream id is required")
	}
	_, errCall := callHost(pluginabi.MethodHostStreamEmit, pluginStreamEmitRequest{StreamID: streamID, Payload: payload})
	return errCall
}

func closePluginStream(streamID string, errMsg string) {
	if strings.TrimSpace(streamID) == "" {
		return
	}
	_, _ = callHost(pluginabi.MethodHostStreamClose, pluginStreamCloseRequest{
		StreamID: streamID,
		Error:    strings.TrimSpace(errMsg),
	})
}

func callHost(method string, payload any) (json.RawMessage, error) {
	rawPayload, errMarshal := json.Marshal(payload)
	if errMarshal != nil {
		return nil, fmt.Errorf("marshal host callback payload %s: %w", method, errMarshal)
	}
	cMethod := C.CString(method)
	defer C.free(unsafe.Pointer(cMethod))
	var response C.cliproxy_buffer
	var requestPtr *C.uint8_t
	if len(rawPayload) > 0 {
		cPayload := C.CBytes(rawPayload)
		if cPayload == nil {
			return nil, fmt.Errorf("allocate host callback payload %s", method)
		}
		defer C.free(cPayload)
		requestPtr = (*C.uint8_t)(cPayload)
	}
	callCode := C.call_host_api(cMethod, requestPtr, C.size_t(len(rawPayload)), &response)
	var rawResponse []byte
	if response.ptr != nil && response.len > 0 {
		rawResponse = C.GoBytes(response.ptr, C.int(response.len))
	}
	if response.ptr != nil {
		C.free_host_buffer(response.ptr, response.len)
	}
	if len(rawResponse) == 0 {
		return nil, fmt.Errorf("host callback %s returned no response, code=%d", method, int(callCode))
	}
	var env envelope
	if errDecode := json.Unmarshal(rawResponse, &env); errDecode != nil {
		return nil, fmt.Errorf("decode host callback envelope %s: %w", method, errDecode)
	}
	if !env.OK {
		if env.Error != nil {
			return nil, fmt.Errorf("%s: %s", env.Error.Code, env.Error.Message)
		}
		return nil, fmt.Errorf("host callback %s failed", method)
	}
	if callCode != 0 {
		return nil, fmt.Errorf("host callback %s returned code=%d", method, int(callCode))
	}
	return append(json.RawMessage(nil), env.Result...), nil
}

func okEnvelope(v any) ([]byte, error) {
	raw, errMarshal := json.Marshal(v)
	if errMarshal != nil {
		return nil, errMarshal
	}
	return json.Marshal(envelope{OK: true, Result: raw})
}

func errorEnvelope(code, message string) []byte {
	raw, _ := json.Marshal(envelope{OK: false, Error: &envelopeError{Code: code, Message: message}})
	return raw
}

func writeResponse(response *C.cliproxy_buffer, raw []byte) {
	if response == nil || len(raw) == 0 {
		return
	}
	ptr := C.CBytes(raw)
	if ptr == nil {
		return
	}
	response.ptr = ptr
	response.len = C.size_t(len(raw))
}
