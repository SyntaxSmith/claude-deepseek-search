package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type httpError struct {
	status int
	msg    string
}

func (e *httpError) Error() string              { return e.msg }
func newHTTPError(status int, msg string) error { return &httpError{status: status, msg: msg} }
func errHTTPStatus(err error) int {
	var e *httpError
	if errors.As(err, &e) {
		return e.status
	}
	return 0
}

func redactDeepSeekError(err error, secret string) error {
	if err == nil {
		return nil
	}
	message := err.Error()
	if secret != "" {
		message = strings.ReplaceAll(message, secret, "[REDACTED]")
	}
	var e *httpError
	if errors.As(err, &e) {
		return newHTTPError(e.status, message)
	}
	return errors.New(message)
}

type deepSeekClient struct {
	APIKey         string
	BaseURL        string
	Count          int
	NeedContent    bool
	TimeoutSeconds int
}

type deepSeekSearchRequest struct {
	Query       string `json:"query"`
	Count       int    `json:"count"`
	NeedContent bool   `json:"need_content"`
}

type deepSeekSearchResponse struct {
	Results []deepSeekSearchResult `json:"results"`
}

type deepSeekSearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
	Summary string `json:"summary"`
	Content string `json:"content"`
	Date    string `json:"date"`
}

type claudeWebSearchHit struct {
	Title   string
	URL     string
	Snippet string
	Date    string
}

type hostHTTPResponse struct {
	StatusCode int                 `json:"StatusCode"`
	Headers    map[string][]string `json:"Headers"`
	Body       []byte              `json:"Body"`
}

type hostHTTPRequest struct {
	HostCallbackID string              `json:"host_callback_id,omitempty"`
	OperationID    string              `json:"operation_id,omitempty"`
	Method         string              `json:"method"`
	URL            string              `json:"url"`
	Headers        map[string][]string `json:"headers,omitempty"`
	Body           []byte              `json:"body,omitempty"`
}

type hostCallResult struct {
	raw json.RawMessage
	err error
}

func (c *deepSeekClient) search(query string, maxResults int, callbackID string) ([]claudeWebSearchHit, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, newHTTPError(http.StatusBadRequest, "web search query is empty")
	}
	if len([]rune(query)) > 128 {
		return nil, newHTTPError(http.StatusBadRequest, "web search query exceeds DeepSeek's 128-character limit")
	}
	if maxResults <= 0 {
		maxResults = c.Count
	}
	if maxResults <= 0 {
		maxResults = 5
	}
	if maxResults > 50 {
		maxResults = 50
	}
	payload, err := json.Marshal(deepSeekSearchRequest{Query: query, Count: maxResults, NeedContent: c.NeedContent})
	if err != nil {
		return nil, fmt.Errorf("marshal DeepSeek search request: %w", err)
	}
	endpoint := strings.TrimRight(c.BaseURL, "/") + "/v1/tools/search"
	request := hostHTTPRequest{
		HostCallbackID: callbackID,
		Method:         http.MethodPost,
		URL:            endpoint,
		Headers: map[string][]string{
			"Authorization": {"Bearer " + c.APIKey},
			"Content-Type":  {"application/json"},
			"Accept":        {"application/json"},
		},
		Body: payload,
	}
	raw, err := c.doHTTP(request, callbackID)
	if err != nil {
		return nil, redactDeepSeekError(err, c.APIKey)
	}
	var response hostHTTPResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, fmt.Errorf("decode DeepSeek HTTP response: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, newHTTPError(response.StatusCode, fmt.Sprintf("DeepSeek Search returned HTTP %d", response.StatusCode))
	}
	var decoded deepSeekSearchResponse
	if err := json.Unmarshal(response.Body, &decoded); err != nil {
		return nil, fmt.Errorf("decode DeepSeek Search response: %w", err)
	}
	hits := make([]claudeWebSearchHit, 0, len(decoded.Results))
	for _, result := range decoded.Results {
		hit := claudeWebSearchHit{
			Title:   strings.TrimSpace(result.Title),
			URL:     strings.TrimSpace(result.URL),
			Snippet: strings.TrimSpace(result.Summary),
			Date:    strings.TrimSpace(result.Date),
		}
		if hit.Snippet == "" {
			hit.Snippet = strings.TrimSpace(result.Snippet)
		}
		if hit.Snippet == "" {
			hit.Snippet = strings.TrimSpace(result.Content)
		}
		if hit.URL != "" {
			hits = append(hits, hit)
		}
	}
	return hits, nil
}

func (c *deepSeekClient) doHTTP(request hostHTTPRequest, callbackID string) (json.RawMessage, error) {
	opened, errOpen := callHost(methodHostHTTPOperationOpen, map[string]string{"host_callback_id": callbackID})
	if errOpen != nil {
		return nil, newHTTPError(http.StatusNotImplemented, "CPA host HTTP cancellation is required; upgrade CPA to a v8 release")
	}
	var operation struct {
		ID string `json:"operation_id"`
	}
	if err := json.Unmarshal(opened, &operation); err != nil || strings.TrimSpace(operation.ID) == "" {
		return nil, fmt.Errorf("DeepSeek HTTP operation returned no operation_id")
	}
	request.OperationID = operation.ID
	result := make(chan hostCallResult, 1)
	go func() {
		raw, err := callHost(methodHostHTTPDo, request)
		result <- hostCallResult{raw, err}
	}()
	select {
	case response := <-result:
		return response.raw, response.err
	case <-time.After(time.Duration(c.TimeoutSeconds) * time.Second):
		_, _ = callHost(methodHostHTTPCancel, map[string]string{
			"host_callback_id": callbackID,
			"operation_id":     operation.ID,
		})
		return nil, newHTTPError(http.StatusGatewayTimeout, "DeepSeek Search request timed out")
	}
}
