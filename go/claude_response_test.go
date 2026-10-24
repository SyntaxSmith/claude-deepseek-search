package main

import (
	"strings"
	"testing"
)

func TestClaudeMessageResponseContainsSearchResult(t *testing.T) {
	builder := newClaudeResponseBuilder("GLM-5.3[1m]")
	raw := builder.buildMessage("latest GLM release", []claudeWebSearchHit{{
		Title: "Release notes", URL: "https://example.com/release", Snippet: "A short summary.",
	}})
	text := string(raw)
	for _, want := range []string{"server_tool_use", "web_search_tool_result", "https://example.com/release", "A short summary."} {
		if !strings.Contains(text, want) {
			t.Fatalf("response does not contain %q: %s", want, text)
		}
	}
}

func TestClaudeStreamResponseIsSSE(t *testing.T) {
	builder := newClaudeResponseBuilder("GLM-5.3")
	raw := builder.buildStream("latest GLM release", nil)
	text := string(raw)
	if !strings.HasPrefix(text, "event: message_start\ndata: ") {
		t.Fatalf("unexpected SSE prefix: %q", text[:min(len(text), 80)])
	}
	if !strings.Contains(text, "event: message_stop\n") {
		t.Fatal("stream is missing message_stop")
	}
}

func TestClaudeFetchResponseContainsDocument(t *testing.T) {
	builder := newClaudeResponseBuilder("GLM-5.3")
	raw := builder.buildFetchMessage(claudeWebFetchPage{
		Title: "Example", URL: "https://example.com/docs", Content: "# Document",
	})
	text := string(raw)
	for _, want := range []string{"web_fetch_tool_result", "web_fetch_result", "https://example.com/docs", "# Document"} {
		if !strings.Contains(text, want) {
			t.Fatalf("response does not contain %q: %s", want, text)
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
