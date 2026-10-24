package main

import (
	"strings"
	"testing"
)

func TestRedactDeepSeekErrorPreservesStatus(t *testing.T) {
	err := redactDeepSeekError(newHTTPError(401, "upstream echoed sk-secret"), "sk-secret")
	if strings.Contains(err.Error(), "sk-secret") {
		t.Fatal("error still contains the API key")
	}
	if got := errHTTPStatus(err); got != 401 {
		t.Fatalf("status = %d, want 401", got)
	}
}
