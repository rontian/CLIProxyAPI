package handlers

import (
	"testing"

	"github.com/tidwall/gjson"
)

func TestParseAutoRouterJudgeResponse(t *testing.T) {
	body := []byte(`{"choices":[{"message":{"content":"{\"role_id\":\"coding\",\"confidence\":0.91,\"reason\":\"debugging\"}"}}]}`)
	decision, err := parseAutoRouterJudgeResponse(body)
	if err != nil {
		t.Fatalf("parseAutoRouterJudgeResponse() error = %v", err)
	}
	if decision.RoleID != "coding" || decision.Confidence != 0.91 || decision.Reason != "debugging" {
		t.Fatalf("decision = %+v, want coding decision", decision)
	}
}

func TestParseAutoRouterJudgeResponseExtractsEmbeddedJSON(t *testing.T) {
	body := []byte(`{"choices":[{"message":{"content":"Here is the decision:\n{\"role\":\"fast\",\"reason\":\"simple\"}"}}]}`)
	decision, err := parseAutoRouterJudgeResponse(body)
	if err != nil {
		t.Fatalf("parseAutoRouterJudgeResponse() error = %v", err)
	}
	if decision.RoleID != "fast" || decision.Reason != "simple" {
		t.Fatalf("decision = %+v, want fast decision", decision)
	}
}

func TestParseAutoRouterJudgeResponseRejectsMissingRole(t *testing.T) {
	if _, err := parseAutoRouterJudgeResponse([]byte(`{"choices":[{"message":{"content":"{\"reason\":\"none\"}"}}]}`)); err == nil {
		t.Fatal("parseAutoRouterJudgeResponse() error = nil, want missing role error")
	}
}

func TestApplyAutoRoutePromptTemplatePrependsOpenAISystemMessage(t *testing.T) {
	body := []byte(`{"model":"auto","messages":[{"role":"user","content":"fix it"}]}`)
	got := applyAutoRoutePromptTemplate("openai", body, modelRouteDecision{PromptTemplate: "You are a coding expert."})
	if role := gjson.GetBytes(got, "messages.0.role").String(); role != "system" {
		t.Fatalf("messages.0.role = %q, want system; body=%s", role, string(got))
	}
	if content := gjson.GetBytes(got, "messages.0.content").String(); content != "You are a coding expert." {
		t.Fatalf("messages.0.content = %q, want prompt", content)
	}
	if content := gjson.GetBytes(got, "messages.1.content").String(); content != "fix it" {
		t.Fatalf("messages.1.content = %q, want original user content", content)
	}
}

func TestApplyAutoRoutePromptTemplatePrependsResponsesInstructions(t *testing.T) {
	body := []byte(`{"model":"auto","instructions":"Be concise.","input":"hello"}`)
	got := applyAutoRoutePromptTemplate("openai-response", body, modelRouteDecision{PromptTemplate: "You are fast."})
	if instructions := gjson.GetBytes(got, "instructions").String(); instructions != "You are fast.\n\nBe concise." {
		t.Fatalf("instructions = %q, want prepended instructions; body=%s", instructions, string(got))
	}
}
