package main

import (
	"encoding/json"
	"testing"
)

func webSearchRequest(t *testing.T) []byte {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"tools": []any{
			map[string]any{"type": claudeWebSearchTypeLatest, "name": "web_search", "max_uses": 5, "allowed_callers": []string{"direct"}},
		},
		"messages": []any{
			map[string]any{"role": "user", "content": "Perform a web search for the query: current GLM release"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestModelAllowlistPreservesNativeSearch(t *testing.T) {
	cfg := pluginConfig{Models: []string{"*"}}
	for _, model := range []string{"GLM-5.3", "Qwen3[1m]"} {
		if !modelAllowed(model, cfg) {
			t.Fatalf("modelAllowed(%q) = false, want true", model)
		}
	}
	for _, model := range []string{"gpt-6.1-sol", "claude-opus-5-5", "deepseek-flash[1m]", "openai/gpt-test", "vendor/deepseek-test"} {
		if modelAllowed(model, cfg) {
			t.Fatalf("modelAllowed(%q) = true, want native model excluded", model)
		}
	}
}

func TestModelAllowlistRequiresExplicitTarget(t *testing.T) {
	if modelAllowed("GLM-5.3", pluginConfig{}) {
		t.Fatal("empty allowlist must not match")
	}
	if modelAllowed("GLM-5.3", pluginConfig{Models: []string{"GLM-*"}, ExcludeModels: []string{"GLM-5.3"}}) {
		t.Fatal("exclude_models must override models")
	}
}

func TestClaudeCodeWebSearchDetection(t *testing.T) {
	body := webSearchRequest(t)
	if !isClaudeCodeBuiltinWebSearchRequest(body, true) {
		t.Fatal("expected Claude Code WebSearch request")
	}
	if got := extractClaudeWebSearchQuery(body); got != "current GLM release" {
		t.Fatalf("query = %q", got)
	}
}

func TestCurrentWebSearchVersionMatches(t *testing.T) {
	body := []byte(`{"tools":[{"type":"web_search_20260318","name":"web_search","allowed_callers":["direct"]}],"messages":[{"role":"user","content":"Perform a web search for the query: current version"}]}`)
	if !isClaudeCodeBuiltinWebSearchRequest(body, true) {
		t.Fatal("web_search_20260318 helper should match")
	}
}

func TestDynamicCallerDefaultIsNotPretendedToBeDirect(t *testing.T) {
	body := []byte(`{"tools":[{"type":"web_search_20260209","name":"web_search"}],"messages":[{"role":"user","content":"Perform a web search for the query: dynamic"}]}`)
	if isClaudeCodeBuiltinWebSearchRequest(body, true) {
		t.Fatal("omitted caller policy must not be treated as direct")
	}
	body = []byte(`{"tools":[{"type":"web_search_20260209","name":"web_search","allowed_callers":"direct"}],"messages":[{"role":"user","content":"Perform a web search for the query: scalar"}]}`)
	if isClaudeCodeBuiltinWebSearchRequest(body, true) {
		t.Fatal("scalar caller policy must not be accepted")
	}
}

func TestMixedToolsDoNotMatchByDefault(t *testing.T) {
	body := []byte(`{"tools":[{"type":"web_search_20260209","name":"web_search","allowed_callers":["direct"]},{"name":"custom_tool","input_schema":{}}]}`)
	if isClaudeCodeBuiltinWebSearchRequest(body, true) {
		t.Fatal("mixed tool request must not match require_web_search_only")
	}
}

func TestOrdinaryWebSearchDeclarationDoesNotMatch(t *testing.T) {
	body := []byte(`{"tools":[{"type":"web_search_20260209","name":"web_search","allowed_callers":["direct"]}],"messages":[{"role":"user","content":"Tell me a joke"}]}`)
	if isClaudeCodeBuiltinWebSearchRequest(body, true) {
		t.Fatal("ordinary Messages request must not match")
	}
}

func TestSearchRestrictionsDoNotMatchSilently(t *testing.T) {
	body := []byte(`{"tools":[{"type":"web_search_20260209","name":"web_search","allowed_callers":["direct"],"blocked_domains":["blocked.example"]}],"messages":[{"role":"user","content":"Perform a web search for the query: news"}]}`)
	if isClaudeCodeBuiltinWebSearchRequest(body, true) {
		t.Fatal("domain-restricted request must remain on a compatible path")
	}
	body = []byte(`{"tools":[{"type":"web_search_20260209","name":"web_search","allowed_callers":["direct"]}],"tool_choice":{"type":"none"},"messages":[{"role":"user","content":"Perform a web search for the query: news"}]}`)
	if isClaudeCodeBuiltinWebSearchRequest(body, true) {
		t.Fatal("tool_choice none must not be intercepted")
	}
	body = []byte(`{"tools":[{"type":"web_search_20260318","name":"web_search","allowed_callers":["direct"],"user_location":{"type":"approximate","country":"CN"}}],"messages":[{"role":"user","content":"Perform a web search for the query: news"}]}`)
	if isClaudeCodeBuiltinWebSearchRequest(body, true) {
		t.Fatal("localized request must remain on a compatible path")
	}
}

func TestSearchQueryUsesLatestHelperTurn(t *testing.T) {
	body := []byte(`{"tools":[{"type":"web_search_20260209","name":"web_search","allowed_callers":["direct"]}],"messages":[{"role":"user","content":"Perform a web search for the query: old"},{"role":"assistant","content":"done"},{"role":"user","content":"Perform a web search for the query: latest"}]}`)
	if got := extractClaudeWebSearchQuery(body); got != "latest" {
		t.Fatalf("query = %q, want latest", got)
	}
}

func TestHistoricalHelperDoesNotHijackContinuation(t *testing.T) {
	body := []byte(`{"tools":[{"type":"web_search_20260209","name":"web_search","allowed_callers":["direct"]}],"messages":[{"role":"user","content":"Perform a web search for the query: old"},{"role":"assistant","content":"done"},{"role":"user","content":"Summarize the results without another search"}]}`)
	if isClaudeCodeBuiltinWebSearchRequest(body, true) {
		t.Fatal("historical helper prompt must not route a continuation")
	}
}
