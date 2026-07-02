package main

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestCopilotStreamFramesUnwrapsSSEData(t *testing.T) {
	raw := []byte("data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n")

	frames := copilotStreamFrames(raw)

	if len(frames) != 1 {
		t.Fatalf("frames len = %d, want 1", len(frames))
	}
	want := []byte("{\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}")
	if !bytes.Equal(frames[0], want) {
		t.Fatalf("frame = %q, want %q", frames[0], want)
	}
}

func TestCopilotStreamFramesKeepsPlainJSONPayload(t *testing.T) {
	raw := []byte(" {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]} ")

	frames := copilotStreamFrames(raw)

	if len(frames) != 1 {
		t.Fatalf("frames len = %d, want 1", len(frames))
	}
	want := []byte("{\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}")
	if !bytes.Equal(frames[0], want) {
		t.Fatalf("frame = %q, want %q", frames[0], want)
	}
}

func TestCopilotStreamFramesHandlesMultipleSSEEvents(t *testing.T) {
	raw := []byte("event: ignored\ndata: {\"a\":1}\n\ndata: [DONE]\n\n")

	frames := copilotStreamFrames(raw)

	if len(frames) != 2 {
		t.Fatalf("frames len = %d, want 2", len(frames))
	}
	if !bytes.Equal(frames[0], []byte("{\"a\":1}")) {
		t.Fatalf("first frame = %q, want {\"a\":1}", frames[0])
	}
	if !bytes.Equal(frames[1], []byte("[DONE]")) {
		t.Fatalf("second frame = %q, want [DONE]", frames[1])
	}
}

func TestCopilotStreamFramerBuffersSplitSSEEvent(t *testing.T) {
	framer := &copilotStreamFramer{}

	first := framer.Feed([]byte("data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hel"))
	if len(first) != 0 {
		t.Fatalf("first feed frames = %q, want none for partial JSON event", first)
	}

	second := framer.Feed([]byte("lo\"}}]}\n\n"))
	if len(second) != 1 {
		t.Fatalf("second feed frames len = %d, want 1", len(second))
	}
	want := []byte("{\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hello\"}}]}")
	if !bytes.Equal(second[0], want) {
		t.Fatalf("second frame = %q, want %q", second[0], want)
	}
	if flushed := framer.Flush(); len(flushed) != 0 {
		t.Fatalf("flush frames = %q, want none", flushed)
	}
}

func TestRewriteModelSanitizesUnsupportedOpenAIExtensionFields(t *testing.T) {
	raw := []byte(`{
		"model":"auto",
		"messages":[{"role":"user","content":"hi"}],
		"stream":false,
		"reasoning_effort":"medium",
		"reasoning":{"effort":"medium"},
		"verbosity":"high",
		"service_tier":"default",
		"store":false,
		"stream_options":{"include_usage":true},
		"metadata":{"client":"zcode"},
		"max_completion_tokens":128,
		"tools":[{"type":"function","function":{"name":"lookup"}}]
	}`)

	rewritten := rewriteModel(raw, "gpt-4o", true)

	var got map[string]any
	if errDecode := json.Unmarshal(rewritten, &got); errDecode != nil {
		t.Fatalf("decode rewritten payload: %v; payload=%s", errDecode, string(rewritten))
	}
	if got["model"] != "gpt-4o" {
		t.Fatalf("model = %v, want gpt-4o", got["model"])
	}
	if got["stream"] != true {
		t.Fatalf("stream = %v, want true", got["stream"])
	}
	if got["max_tokens"].(float64) != 128 {
		t.Fatalf("max_tokens = %v, want 128", got["max_tokens"])
	}
	for _, key := range []string{"reasoning_effort", "reasoning", "verbosity", "service_tier", "store", "stream_options", "metadata", "max_completion_tokens"} {
		if _, exists := got[key]; exists {
			t.Fatalf("unexpected %s in rewritten payload: %s", key, string(rewritten))
		}
	}
	if _, exists := got["tools"]; !exists {
		t.Fatalf("tools should be preserved: %s", string(rewritten))
	}
}
