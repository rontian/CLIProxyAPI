package management

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/registry"
)

func TestGetStaticModelDefinitionsFallsBackToRuntimeProviderModels(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const provider = "test-runtime-provider"
	registryRef := registry.GetGlobalRegistry()
	registryRef.RegisterClient("test-runtime-provider-client", provider, []*registry.ModelInfo{
		{ID: "runtime-model-1", DisplayName: "Runtime Model One"},
	})
	t.Cleanup(func() {
		registryRef.UnregisterClient("test-runtime-provider-client")
	})

	handler := &Handler{}
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Params = gin.Params{{Key: "channel", Value: provider}}
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v0/management/model-definitions/"+provider, nil)

	handler.GetStaticModelDefinitions(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var body struct {
		Channel string                `json:"channel"`
		Models  []*registry.ModelInfo `json:"models"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Channel != provider {
		t.Fatalf("channel = %q, want %q", body.Channel, provider)
	}
	if len(body.Models) != 1 || body.Models[0].ID != "runtime-model-1" {
		t.Fatalf("models = %#v, want runtime-model-1", body.Models)
	}
}

func TestGetStaticModelDefinitionsUnknownChannelReturnsNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := &Handler{}
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Params = gin.Params{{Key: "channel", Value: "missing-runtime-provider"}}
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v0/management/model-definitions/missing-runtime-provider", nil)

	handler.GetStaticModelDefinitions(ctx)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}
