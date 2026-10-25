package main

import (
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/match"
)

const (
	claudeWebSearchTypeLegacy  = "web_search_20250305"
	claudeWebSearchTypeLatest  = "web_search_20260209"
	claudeWebSearchTypeCurrent = "web_search_20260318"
	claudeWebFetchTypeLegacy   = "web_fetch_20250910"
	claudeWebFetchTypeLatest   = "web_fetch_20260209"
	claudeWebFetchTypeCache    = "web_fetch_20260309"
	claudeWebFetchTypeCurrent  = "web_fetch_20260318"
)

// Keep provider-native search models on their native path.
// DeepSeek is intentionally absent: its chat API has no server-side search, so it must stay interceptable.
var nativeSearchModelPatterns = []string{
	"gpt*", "o1*", "o3*", "o4*", "o5*",
	"claude*",
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

func isClaudeWebFetchType(toolType string) bool {
	return toolType == claudeWebFetchTypeLegacy || toolType == claudeWebFetchTypeLatest || toolType == claudeWebFetchTypeCache || toolType == claudeWebFetchTypeCurrent
}

func hasClaudeWebFetchTool(body []byte) bool {
	for _, tool := range gjson.GetBytes(body, "tools").Array() {
		if isClaudeWebFetchType(tool.Get("type").String()) {
			return true
		}
	}
	return false
}

func hasOnlyClaudeWebFetchTools(body []byte) bool {
	tools := gjson.GetBytes(body, "tools")
	if !tools.IsArray() {
		return false
	}
	hasFetch := false
	for _, tool := range tools.Array() {
		toolType := tool.Get("type").String()
		if isClaudeWebFetchType(toolType) {
			hasFetch = true
			continue
		}
		if toolType != "" || tool.Get("name").String() != "" {
			return false
		}
	}
	return hasFetch
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

const webSearchPromptPrefix = "perform a web search for the query:"

func latestUserText(body []byte) string {
	messages := gjson.GetBytes(body, "messages").Array()
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Get("role").String() == "user" {
			return strings.TrimSpace(extractClaudeMessageText(messages[i].Get("content")))
		}
	}
	return ""
}

func webSearchPromptQuery(text string) (string, bool) {
	text = strings.TrimSpace(text)
	lower := strings.ToLower(text)
	if !strings.HasPrefix(lower, webSearchPromptPrefix) {
		return "", false
	}
	return strings.TrimSpace(text[len(webSearchPromptPrefix):]), true
}

func looksLikeClaudeCodeWebSearchAssistant(body []byte) bool {
	_, ok := webSearchPromptQuery(latestUserText(body))
	return ok
}

func isClaudeCodeBuiltinWebSearchRequest(body []byte, requireOnly bool) bool {
	if gjson.GetBytes(body, "tool_choice.type").String() == "tool" &&
		gjson.GetBytes(body, "tool_choice.name").String() != "web_search" {
		return false
	}
	if !hasClaudeWebSearchTool(body) || hasToolChoiceNone(body) || hasUnsupportedSearchOptions(body) {
		return false
	}
	if requireOnly && !hasOnlyClaudeWebSearchTools(body) {
		return false
	}
	return looksLikeClaudeCodeWebSearchAssistant(body)
}

func isDirectWebFetchRequest(body []byte, requireOnly bool) bool {
	if gjson.GetBytes(body, "tool_choice.type").String() != "tool" ||
		gjson.GetBytes(body, "tool_choice.name").String() != "web_fetch" {
		return false
	}
	if !hasClaudeWebFetchTool(body) || hasUnsupportedFetchOptions(body) {
		return false
	}
	if requireOnly && !hasOnlyClaudeWebFetchTools(body) {
		return false
	}
	return extractClaudeWebFetchURL(body) != ""
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

func hasUnsupportedFetchOptions(body []byte) bool {
	for _, tool := range gjson.GetBytes(body, "tools").Array() {
		if !isClaudeWebFetchType(tool.Get("type").String()) {
			continue
		}
		for _, key := range []string{"allowed_domains", "blocked_domains", "url_sources", "citations", "max_content_tokens", "use_cache"} {
			if tool.Get(key).Exists() {
				return true
			}
		}
		if tool.Get("response_inclusion").String() == "excluded" || unsupportedCallerPolicy(tool) {
			return true
		}
	}
	return false
}

func extractClaudeWebFetchURL(body []byte) string {
	messages := gjson.GetBytes(body, "messages").Array()
	if len(messages) == 0 || messages[len(messages)-1].Get("role").String() != "user" {
		return ""
	}
	return extractHTTPURL(extractClaudeMessageText(messages[len(messages)-1].Get("content")))
}

func extractHTTPURL(text string) string {
	start := -1
	for _, prefix := range []string{"https://", "http://"} {
		if i := strings.Index(text, prefix); i >= 0 && (start < 0 || i < start) {
			start = i
		}
	}
	if start < 0 {
		return ""
	}
	url := text[start:]
	if end := strings.IndexAny(url, " \t\r\n\"'<>"); end >= 0 {
		url = url[:end]
	}
	url = strings.TrimRight(url, ".,;:")
	for strings.HasSuffix(url, ")") && strings.Count(url, ")") > strings.Count(url, "(") {
		url = strings.TrimSuffix(url, ")")
	}
	return url
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
	// Dynamic versions default to code-execution callers when omitted.
	switch tool.Get("type").String() {
	case claudeWebSearchTypeLatest, claudeWebSearchTypeCurrent,
		claudeWebFetchTypeLatest, claudeWebFetchTypeCache, claudeWebFetchTypeCurrent:
		return true
	default:
		return false
	}
}

func extractClaudeWebSearchQuery(body []byte) string {
	messages := gjson.GetBytes(body, "messages").Array()
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Get("role").String() != "user" {
			continue
		}
		if query, ok := webSearchPromptQuery(extractClaudeMessageText(messages[i].Get("content"))); ok {
			return query
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
		pattern = strings.ToLower(strings.TrimSpace(pattern))
		pattern = strings.ReplaceAll(pattern, `\`, `\\`)
		if match.Match(value, pattern) {
			return true
		}
	}
	return false
}
