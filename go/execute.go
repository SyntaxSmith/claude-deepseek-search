package main

func executeWebRequest(req executorRequest) ([]byte, error) {
	cfg := loadedConfig()
	body := req.OriginalRequest
	if len(body) == 0 {
		body = req.Payload
	}
	if isClaudeCodeBuiltinWebSearchRequest(body, cfg.RequireWebSearchOnly) {
		return executeSearch(req)
	}
	if isDirectWebFetchRequest(body, cfg.RequireWebSearchOnly) {
		return executeFetch(req, body)
	}
	return nil, newHTTPError(400, "unsupported web tool request")
}

func executeFetch(req executorRequest, body []byte) ([]byte, error) {
	url := extractClaudeWebFetchURL(body)
	if url == "" {
		return nil, newHTTPError(400, "web fetch URL is empty")
	}
	cfg := loadedConfig()
	client := &deepSeekClient{
		APIKey:         resolveAPIKey(cfg),
		BaseURL:        cfg.BaseURL,
		TimeoutSeconds: cfg.TimeoutSeconds,
	}
	page, err := client.fetch(url, req.HostCallbackID)
	if err != nil {
		return nil, err
	}
	builder := newClaudeResponseBuilder(req.Model)
	if req.Stream {
		return builder.buildFetchStream(page), nil
	}
	return builder.buildFetchMessage(page), nil
}
