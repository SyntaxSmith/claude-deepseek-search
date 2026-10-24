package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"time"
)

type claudeResponseBuilder struct {
	model     string
	messageID string
	toolID    string
}

var responseSequence atomic.Uint64

func newClaudeResponseBuilder(model string) *claudeResponseBuilder {
	model = strings.TrimSpace(model)
	if model == "" {
		model = "unknown"
	}
	sequence := responseSequence.Add(1)
	stamp := time.Now().UnixNano()
	return &claudeResponseBuilder{
		model:     model,
		messageID: fmt.Sprintf("msg_deepseek_%x_%d", stamp, sequence),
		toolID:    fmt.Sprintf("srvtoolu_deepseek_%x_%d", stamp, sequence),
	}
}

func (b *claudeResponseBuilder) buildMessage(query string, hits []claudeWebSearchHit) []byte {
	text := composeSearchText(hits)
	content := []map[string]any{
		{"type": "server_tool_use", "id": b.toolID, "name": "web_search", "input": map[string]string{"query": query}},
		{"type": "web_search_tool_result", "tool_use_id": b.toolID, "content": webSearchResultBlocks(hits)},
		{"type": "text", "text": text},
	}
	payload := map[string]any{
		"id": b.messageID, "type": "message", "role": "assistant", "model": b.model,
		"content": content, "stop_reason": "end_turn", "stop_sequence": nil,
		"usage": map[string]any{
			"input_tokens": 1, "output_tokens": estimateTokens(text),
			"server_tool_use": map[string]any{"web_search_requests": 1},
		},
	}
	raw, _ := json.Marshal(payload)
	return raw
}

func (b *claudeResponseBuilder) buildStream(query string, hits []claudeWebSearchHit) []byte {
	text := composeSearchText(hits)
	var chunks []string
	chunks = append(chunks, b.event("message_start", map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id": b.messageID, "type": "message", "role": "assistant", "content": []any{},
			"model": b.model, "stop_reason": nil, "stop_sequence": nil,
			"usage": map[string]any{"input_tokens": 1, "output_tokens": 0},
		},
	}))
	chunks = append(chunks, b.blockStart(0, map[string]any{
		"type": "server_tool_use", "id": b.toolID, "name": "web_search", "input": map[string]any{},
	}))
	input, _ := json.Marshal(map[string]string{"query": query})
	chunks = append(chunks, b.event("content_block_delta", map[string]any{
		"type": "content_block_delta", "index": 0,
		"delta": map[string]any{"type": "input_json_delta", "partial_json": string(input)},
	}))
	chunks = append(chunks, b.event("content_block_stop", map[string]any{"type": "content_block_stop", "index": 0}))
	chunks = append(chunks, b.blockStart(1, map[string]any{
		"type": "web_search_tool_result", "tool_use_id": b.toolID, "content": webSearchResultBlocks(hits),
	}))
	chunks = append(chunks, b.event("content_block_stop", map[string]any{"type": "content_block_stop", "index": 1}))
	chunks = append(chunks, b.blockStart(2, map[string]any{"type": "text", "text": ""}))
	chunks = append(chunks, b.event("content_block_delta", map[string]any{
		"type": "content_block_delta", "index": 2,
		"delta": map[string]any{"type": "text_delta", "text": text},
	}))
	chunks = append(chunks, b.event("content_block_stop", map[string]any{"type": "content_block_stop", "index": 2}))
	chunks = append(chunks, b.event("message_delta", map[string]any{
		"type": "message_delta", "delta": map[string]any{"stop_reason": "end_turn", "stop_sequence": nil},
		"usage": map[string]any{
			"input_tokens": 1, "output_tokens": estimateTokens(text),
			"server_tool_use": map[string]any{"web_search_requests": 1},
		},
	}))
	chunks = append(chunks, b.event("message_stop", map[string]any{"type": "message_stop"}))
	return []byte(strings.Join(chunks, ""))
}

func webSearchResultBlocks(hits []claudeWebSearchHit) []map[string]any {
	blocks := make([]map[string]any, 0, len(hits))
	for _, hit := range hits {
		title := hit.Title
		if title == "" {
			title = hostFromURL(hit.URL)
		}
		pageAge := any(nil)
		if hit.Date != "" {
			pageAge = hit.Date
		}
		blocks = append(blocks, map[string]any{
			"type": "web_search_result", "title": title, "url": hit.URL,
			"encrypted_content": encryptedSearchContext(hit), "page_age": pageAge,
		})
	}
	return blocks
}

// Claude Code replay context; Base64 JSON, not ciphertext.
func encryptedSearchContext(hit claudeWebSearchHit) string {
	raw, _ := json.Marshal(map[string]string{
		"version": "1",
		"title":   hit.Title,
		"url":     hit.URL,
		"snippet": hit.Snippet,
	})
	return base64.StdEncoding.EncodeToString(raw)
}

func composeSearchText(hits []claudeWebSearchHit) string {
	if len(hits) == 0 {
		return "No web search results were returned."
	}
	var out strings.Builder
	for i, hit := range hits {
		if i > 0 {
			out.WriteString("\n\n")
		}
		if hit.Title != "" {
			out.WriteString(hit.Title)
			out.WriteString("\n")
		}
		if hit.URL != "" {
			out.WriteString(hit.URL)
			out.WriteString("\n")
		}
		if hit.Snippet != "" {
			out.WriteString(hit.Snippet)
		}
	}
	return out.String()
}

func (b *claudeResponseBuilder) event(eventType string, data map[string]any) string {
	raw, _ := json.Marshal(data)
	return fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, raw)
}

func (b *claudeResponseBuilder) blockStart(index int, block map[string]any) string {
	return b.event("content_block_start", map[string]any{
		"type": "content_block_start", "index": index, "content_block": block,
	})
}

func hostFromURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if index := strings.Index(raw, "://"); index >= 0 {
		raw = raw[index+3:]
	}
	if index := strings.IndexByte(raw, '/'); index >= 0 {
		return raw[:index]
	}
	return raw
}

func estimateTokens(text string) int {
	tokens := len([]rune(text)) / 4
	if tokens < 1 {
		return 1
	}
	return tokens
}
