package pluginhost

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/runtime/executor/helps"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/proxyutil"
	log "github.com/sirupsen/logrus"
)

type hostHTTPClient struct {
	host     *Host
	auth     *coreauth.Auth
	provider string
}

func (h *Host) newHTTPClient(auth *coreauth.Auth, providers ...string) pluginapi.HostHTTPClient {
	provider := ""
	if len(providers) > 0 {
		provider = providers[0]
	}
	return &hostHTTPClient{host: h, auth: auth, provider: provider}
}

func (c *hostHTTPClient) Do(ctx context.Context, req pluginapi.HTTPRequest) (pluginapi.HTTPResponse, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	resp, cfg, errDo := c.doHTTP(ctx, req)
	if errDo != nil {
		return pluginapi.HTTPResponse{}, errDo
	}
	defer func() {
		if errClose := resp.Body.Close(); errClose != nil {
			log.Warnf("pluginhost: response body close error: %v", errClose)
		}
	}()
	helps.RecordAPIResponseMetadata(ctx, cfg, resp.StatusCode, resp.Header.Clone())
	body, errReadAll := io.ReadAll(resp.Body)
	if len(body) > 0 {
		helps.AppendAPIResponseChunk(ctx, cfg, body)
	}
	if errReadAll != nil {
		helps.RecordAPIResponseError(ctx, cfg, errReadAll)
		return pluginapi.HTTPResponse{}, fmt.Errorf("read host http response: %w", errReadAll)
	}
	return pluginapi.HTTPResponse{
		StatusCode: resp.StatusCode,
		Headers:    cloneHeader(resp.Header),
		Body:       body,
	}, nil
}

func (c *hostHTTPClient) DoStream(ctx context.Context, req pluginapi.HTTPRequest) (pluginapi.HTTPStreamResponse, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	resp, cfg, errDo := c.doHTTP(ctx, req)
	if errDo != nil {
		return pluginapi.HTTPStreamResponse{}, errDo
	}
	helps.RecordAPIResponseMetadata(ctx, cfg, resp.StatusCode, resp.Header.Clone())
	chunks := make(chan pluginapi.HTTPStreamChunk)
	go func() {
		defer close(chunks)
		defer func() {
			if errClose := resp.Body.Close(); errClose != nil {
				log.Warnf("pluginhost: stream response body close error: %v", errClose)
			}
		}()
		buf := make([]byte, 32*1024)
		for {
			n, errRead := resp.Body.Read(buf)
			if n > 0 {
				payload := bytes.Clone(buf[:n])
				helps.AppendAPIResponseChunk(ctx, cfg, payload)
				select {
				case <-ctx.Done():
					return
				case chunks <- pluginapi.HTTPStreamChunk{Payload: payload}:
				}
			}
			if errRead != nil {
				if errRead != io.EOF {
					helps.RecordAPIResponseError(ctx, cfg, errRead)
					select {
					case <-ctx.Done():
					case chunks <- pluginapi.HTTPStreamChunk{Err: errRead}:
					}
				}
				return
			}
		}
	}()
	return pluginapi.HTTPStreamResponse{
		StatusCode: resp.StatusCode,
		Headers:    cloneHeader(resp.Header),
		Chunks:     chunks,
	}, nil
}

func (c *hostHTTPClient) doHTTP(ctx context.Context, req pluginapi.HTTPRequest) (*http.Response, *config.Config, error) {
	if c == nil || c.host == nil {
		return nil, nil, fmt.Errorf("host http client is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	cfg := c.host.currentRuntimeConfig()
	method := req.Method
	if method == "" {
		method = http.MethodGet
	}
	httpReq, errNewRequest := http.NewRequestWithContext(ctx, method, req.URL, bytes.NewReader(bytes.Clone(req.Body)))
	if errNewRequest != nil {
		return nil, cfg, fmt.Errorf("create host http request: %w", errNewRequest)
	}
	httpReq.Header = cloneHeader(req.Headers)
	c.recordHTTPRequest(ctx, cfg, httpReq, req.Body)
	client := c.newRequestHTTPClient(ctx, cfg, req.ProxyURL)
	if client == nil {
		client = &http.Client{}
	}
	resp, errDo := client.Do(httpReq)
	if errDo != nil {
		helps.RecordAPIResponseError(ctx, cfg, errDo)
		c.logHTTPRequestError(cfg, httpReq, req.ProxyURL, errDo)
		return nil, cfg, fmt.Errorf("execute host http request: %w", errDo)
	}
	return resp, cfg, nil
}

func (c *hostHTTPClient) newRequestHTTPClient(ctx context.Context, cfg *config.Config, requestProxyURL string) *http.Client {
	requestProxyURL = strings.TrimSpace(requestProxyURL)
	if requestProxyURL == "" {
		return helps.NewProxyAwareHTTPClient(ctx, cfg, c.auth, 0)
	}
	return helps.NewProxyAwareHTTPClient(ctx, cfg, &coreauth.Auth{ProxyURL: requestProxyURL}, 0)
}

func (c *hostHTTPClient) logHTTPRequestError(cfg *config.Config, req *http.Request, requestProxyURL string, err error) {
	if req == nil {
		return
	}
	fields := log.Fields{
		"method": req.Method,
		"url":    safeRequestURLForLog(req),
	}
	for key, value := range c.proxyLogFields(cfg, requestProxyURL) {
		fields[key] = value
	}
	log.WithFields(fields).WithError(err).Warn("pluginhost: host http request failed")
}

func (c *hostHTTPClient) proxyLogFields(cfg *config.Config, requestProxyURL string) log.Fields {
	proxyURL := ""
	source := "inherit"
	requestProxyURL = strings.TrimSpace(requestProxyURL)
	if requestProxyURL != "" {
		proxyURL = requestProxyURL
		source = "request"
	}
	if c != nil && c.auth != nil {
		authProxyURL := strings.TrimSpace(c.auth.ProxyURL)
		if proxyURL == "" && authProxyURL != "" {
			proxyURL = authProxyURL
			source = "auth"
		}
	}
	if proxyURL == "" && cfg != nil {
		proxyURL = strings.TrimSpace(cfg.ProxyURL)
		if proxyURL != "" {
			source = "config"
		}
	}

	setting, errParse := proxyutil.Parse(proxyURL)
	fields := log.Fields{
		"proxy_source": source,
		"proxy_url":    proxyURLForLog(proxyURL, setting.Mode),
	}
	fields["proxy_mode"] = proxyModeForLog(setting.Mode)
	if errParse != nil {
		fields["proxy_error"] = errParse.Error()
	}
	return fields
}

func proxyURLForLog(proxyURL string, mode proxyutil.Mode) string {
	switch mode {
	case proxyutil.ModeInherit:
		return ""
	case proxyutil.ModeDirect:
		return strings.TrimSpace(proxyURL)
	case proxyutil.ModeProxy:
		return proxyutil.Redact(proxyURL)
	default:
		return proxyutil.Redact(proxyURL)
	}
}

func proxyModeForLog(mode proxyutil.Mode) string {
	switch mode {
	case proxyutil.ModeInherit:
		return "inherit"
	case proxyutil.ModeDirect:
		return "direct"
	case proxyutil.ModeProxy:
		return "proxy"
	case proxyutil.ModeInvalid:
		return "invalid"
	default:
		return "unknown"
	}
}

func safeRequestURLForLog(req *http.Request) string {
	if req == nil || req.URL == nil {
		return ""
	}
	out := *req.URL
	out.RawQuery = ""
	out.Fragment = ""
	return out.String()
}

func (c *hostHTTPClient) recordHTTPRequest(ctx context.Context, cfg *config.Config, req *http.Request, body []byte) {
	if req == nil {
		return
	}
	provider := c.provider
	var authID, authLabel, authType, authValue string
	if c.auth != nil {
		authID = c.auth.ID
		authLabel = c.auth.Label
		authType, authValue = c.auth.AccountInfo()
		if provider == "" {
			provider = c.auth.Provider
		}
	}
	helps.RecordAPIRequest(ctx, cfg, helps.UpstreamRequestLog{
		URL:       req.URL.String(),
		Method:    req.Method,
		Headers:   req.Header.Clone(),
		Body:      bytes.Clone(body),
		Provider:  provider,
		AuthID:    authID,
		AuthLabel: authLabel,
		AuthType:  authType,
		AuthValue: authValue,
	})
}

func (h *Host) currentRuntimeConfig() *config.Config {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.runtimeConfig
}
