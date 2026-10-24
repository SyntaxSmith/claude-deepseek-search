package main

import (
	"encoding/json"
	"testing"
)

func TestFetchDoesNotReuseHistoricalURL(t *testing.T) {
	body := []byte(`{"tools":[{"type":"web_fetch_20250910","name":"web_fetch"}],"tool_choice":{"type":"tool","name":"web_fetch"},"messages":[{"role":"assistant","content":"https://example.com/old"},{"role":"user","content":"Tell me a joke"}]}`)
	if isDirectWebFetchRequest(body, true) {
		t.Fatal("historical URL must not trigger Fetch")
	}
}

func TestFetchURLPreservesParentheses(t *testing.T) {
	want := "https://en.wikipedia.org/wiki/Go_(programming_language)"
	for _, input := range []string{"Fetch " + want, "Read [Go](" + want + ")."} {
		if got := extractHTTPURL(input); got != want {
			t.Fatalf("URL = %q, want %q", got, want)
		}
	}
}

func TestSearchWithFetchDeclarationKeepsSearchRoute(t *testing.T) {
	body := []byte(`{"tools":[{"type":"web_search_20250305","name":"web_search"},{"type":"web_fetch_20250910","name":"web_fetch","blocked_domains":["example.com"]}],"tool_choice":{"type":"tool","name":"web_search"},"messages":[{"role":"user","content":"Perform a web search for the query: https://example.com"}]}`)
	if !isClaudeCodeBuiltinWebSearchRequest(body, false) || isDirectWebFetchRequest(body, false) {
		t.Fatal("Search helper must not be dispatched to Fetch")
	}
}

func TestFetchMessageCarriesDocument(t *testing.T) {
	page := claudeWebFetchPage{Title: "Test", URL: "https://example.com", Content: "Page content"}
	builder := newClaudeResponseBuilder("GLM-test")
	var message struct {
		Content []map[string]any `json:"content"`
	}
	if err := json.Unmarshal(builder.buildFetchMessage(page), &message); err != nil {
		t.Fatal(err)
	}
	result := message.Content[1]["content"].(map[string]any)
	doc := result["content"].(map[string]any)
	if result["type"] != "web_fetch_result" || doc["source"].(map[string]any)["data"] != page.Content {
		t.Fatalf("unexpected Fetch document: %#v", result)
	}
}
