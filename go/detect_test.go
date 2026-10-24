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
	if modelAllowed(`foo\bar`, pluginConfig{Models: []string{"*"}, ExcludeModels: []string{`foo\bar`}}) {
		t.Fatal("backslashes in patterns must remain literal")
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

func TestDirectWebFetchDetection(t *testing.T) {
	body := []byte(`{"tools":[{"type":"web_fetch_20250910","name":"web_fetch"}],"tool_choice":{"type":"tool","name":"web_fetch"},"messages":[{"role":"user","content":"Fetch https://example.com/docs and summarize it"}]}`)
	if !isDirectWebFetchRequest(body, true) {
		t.Fatal("expected direct API Fetch request")
	}
	if got := extractClaudeWebFetchURL(body); got != "https://example.com/docs" {
		t.Fatalf("url = %q", got)
	}
}

func TestDynamicWebFetchAndOptionsDoNotMatch(t *testing.T) {
	body := []byte(`{"tools":[{"type":"web_fetch_20260209","name":"web_fetch"}],"tool_choice":{"type":"tool","name":"web_fetch"},"messages":[{"role":"user","content":"Fetch https://example.com/docs"}]}`)
	if isDirectWebFetchRequest(body, true) {
		t.Fatal("omitted dynamic caller policy must not match")
	}
	body = []byte(`{"tools":[{"type":"web_fetch_20250910","name":"web_fetch","citations":{"enabled":true}}],"tool_choice":{"type":"tool","name":"web_fetch"},"messages":[{"role":"user","content":"Fetch https://example.com/docs"}]}`)
	if isDirectWebFetchRequest(body, true) {
		t.Fatal("unsupported fetch options must not match")
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
