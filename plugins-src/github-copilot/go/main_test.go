package main

import (
	"bytes"
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
