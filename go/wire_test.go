package main

import (
	"encoding/json"
	"testing"
)

func TestExecutorRequestAcceptsSnakeCaseOriginalRequest(t *testing.T) {
	var request executorRequest
	if err := json.Unmarshal([]byte(`{"original_request":"e30=","host_callback_id":"cb","stream_id":"stream"}`), &request); err != nil {
		t.Fatal(err)
	}
	if string(request.OriginalRequest) != "{}" || request.HostCallbackID != "cb" || request.StreamID != "stream" {
		t.Fatalf("request = %#v", request)
	}
}
