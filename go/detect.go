package main

import (
	"strings"

	"github.com/tidwall/gjson"
)

const (
	claudeWebSearchTypeLegacy  = "web_search_20250305"
	claudeWebSearchTypeLatest  = "web_search_20260209"
	claudeWebSearchTypeCurrent = "web_search_20260318"
)

// These are never intercepted even when a user uses a broad allowlist such as "*".
// The user can still keep the list narrow and avoid relying on these defaults.
var nativeSearchModelPatterns = []string{
	"gpt*", "o1*", "o3*", "o4*", "o5*",
	"claude*", "deepseek*",
}

func isClaudeSourceFormat(format string) bool {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "claude", "anthropic":
		return true
	default:
		return false
	}
}

func isClaudeWebSearchType(toolType string) bool {
	return toolType == claudeWebSearchTypeLegacy || toolType == claudeWebSearchTypeLatest || toolType == claudeWebSearchTypeCurrent
}

func hasClaudeWebSearchTool(body []byte) bool {
	for _, tool := range gjson.GetBytes(body, "tools").Array() {
		if isClaudeWebSearchType(tool.Get("type").String()) {
			return true
		}
	}
	return false
}

func hasOnlyClaudeWebSearchTools(body []byte) bool {
	tools := gjson.GetBytes(body, "tools")
	if !tools.IsArray() {
		return false
	}
	hasSearch := false
	for _, tool := range tools.Array() {
		toolType := tool.Get("type").String()
		if isClaudeWebSearchType(toolType) {
			hasSearch = true
			continue
		}
		if toolType != "" || tool.Get("name").String() != "" {
			return false
		}
	}
	return hasSearch
}

func looksLikeClaudeCodeWebSearchAssistant(body []byte) bool {
	const prefix = "perform a web search for the query:"
	messages := gjson.GetBytes(body, "messages").Array()
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Get("role").String() != "user" {
			continue
		}
		text := strings.ToLower(strings.TrimSpace(extractClaudeMessageText(messages[i].Get("content"))))
		return strings.HasPrefix(text, prefix)
	}
	return false
}

func isClaudeCodeBuiltinWebSearchRequest(body []byte, requireOnly bool) bool {
	if !hasClaudeWebSearchTool(body) || hasToolChoiceNone(body) || hasUnsupportedSearchOptions(body) {
		return false
	}
	if requireOnly && !hasOnlyClaudeWebSearchTools(body) {
		return false
	}
	// The helper prompt is the positive Claude Code signal. An ordinary
	// Messages request with only a web_search declaration must not match.
	return looksLikeClaudeCodeWebSearchAssistant(body)
}

func hasToolChoiceNone(body []byte) bool {
	return strings.EqualFold(gjson.GetBytes(body, "tool_choice.type").String(), "none")
}

func hasUnsupportedSearchOptions(body []byte) bool {
	for _, tool := range gjson.GetBytes(body, "tools").Array() {
		if !isClaudeWebSearchType(tool.Get("type").String()) {
			continue
		}
		if len(tool.Get("allowed_domains").Array()) > 0 || len(tool.Get("blocked_domains").Array()) > 0 {
			return true
		}
		if location := tool.Get("user_location"); location.Exists() && location.Type != gjson.Null {
			return true
		}
		if unsupportedCallerPolicy(tool) {
			return true
		}
		if tool.Get("response_inclusion").String() == "excluded" {
			return true
		}
	}
	return false
}

func unsupportedCallerPolicy(tool gjson.Result) bool {
	callers := tool.Get("allowed_callers")
	if callers.Exists() {
		if !callers.IsArray() {
			return true
		}
		values := callers.Array()
		return len(values) != 1 || values[0].String() != "direct"
	}
	// These versions default to dynamic code-execution callers when omitted.
	return tool.Get("type").String() == claudeWebSearchTypeLatest || tool.Get("type").String() == claudeWebSearchTypeCurrent
}

func extractClaudeWebSearchQuery(body []byte) string {
	const prefix = "perform a web search for the query:"
	messages := gjson.GetBytes(body, "messages").Array()
	for i := len(messages) - 1; i >= 0; i-- {
		message := messages[i]
		if message.Get("role").String() != "user" {
			continue
		}
		text := strings.TrimSpace(extractClaudeMessageText(message.Get("content")))
		if len(text) >= len(prefix) && strings.EqualFold(text[:len(prefix)], prefix) {
			return strings.TrimSpace(text[len(prefix):])
		}
	}
	for i := len(messages) - 1; i >= 0; i-- {
		if role := messages[i].Get("role").String(); role != "" && role != "user" {
			continue
		}
		if text := strings.TrimSpace(extractClaudeMessageText(messages[i].Get("content"))); text != "" {
			return text
		}
	}
	return ""
}

func extractClaudeMessageText(content gjson.Result) string {
	if content.Type == gjson.String {
		return content.String()
	}
	var parts []string
	for _, block := range content.Array() {
		if block.Get("type").String() != "text" {
			continue
		}
		if text := strings.TrimSpace(block.Get("text").String()); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}

func modelAllowed(model string, cfg pluginConfig) bool {
	identities := modelIdentities(model)
	if len(identities) == 0 || !matchesAnyIdentity(identities, cfg.Models) {
		return false
	}
	if matchesAnyIdentity(identities, cfg.ExcludeModels) || matchesAnyIdentity(identities, nativeSearchModelPatterns) {
		return false
	}
	return true
}

func modelIdentities(model string) []string {
	model = strings.ToLower(strings.TrimSpace(model))
	if model == "" {
		return nil
	}
	identities := []string{model}
	if index := strings.LastIndexByte(model, '/'); index >= 0 && index+1 < len(model) {
		identities = append(identities, model[index+1:])
	}
	return identities
}

func matchesAnyIdentity(identities, patterns []string) bool {
	for _, identity := range identities {
		if matchesAnyPattern(identity, patterns) {
			return true
		}
	}
	return false
}

func matchesAnyPattern(value string, patterns []string) bool {
	for _, pattern := range patterns {
		if wildcardMatch(strings.ToLower(strings.TrimSpace(pattern)), value) {
			return true
		}
	}
	return false
}

func wildcardMatch(pattern, value string) bool {
	p, v := []rune(pattern), []rune(value)
	dp := make([][]bool, len(p)+1)
	for i := range dp {
		dp[i] = make([]bool, len(v)+1)
	}
	dp[0][0] = true
	for i := 1; i <= len(p); i++ {
		if p[i-1] == '*' {
			dp[i][0] = dp[i-1][0]
		}
		for j := 1; j <= len(v); j++ {
			if p[i-1] == '*' {
				dp[i][j] = dp[i-1][j] || dp[i][j-1]
				continue
			}
			dp[i][j] = dp[i-1][j-1] && (p[i-1] == '?' || p[i-1] == v[j-1])
		}
	}
	return dp[len(p)][len(v)]
}
