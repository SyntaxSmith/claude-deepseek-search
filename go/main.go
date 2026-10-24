package main

/*
#include <stdint.h>
#include <stdlib.h>

typedef struct { void* ptr; size_t len; } cliproxy_buffer;
typedef int (*cliproxy_host_call_fn)(void*, const char*, const uint8_t*, size_t, cliproxy_buffer*);
typedef void (*cliproxy_host_free_fn)(void*, size_t);
typedef struct {
	uint32_t abi_version;
	void* host_ctx;
	cliproxy_host_call_fn call;
	cliproxy_host_free_fn free_buffer;
} cliproxy_host_api;
typedef int (*cliproxy_plugin_call_fn)(char*, uint8_t*, size_t, cliproxy_buffer*);
typedef void (*cliproxy_plugin_free_fn)(void*, size_t);
typedef void (*cliproxy_plugin_shutdown_fn)(void);
typedef struct {
	uint32_t abi_version;
	cliproxy_plugin_call_fn call;
	cliproxy_plugin_free_fn free_buffer;
	cliproxy_plugin_shutdown_fn shutdown;
} cliproxy_plugin_api;

extern int cliproxyPluginCall(char*, uint8_t*, size_t, cliproxy_buffer*);
extern void cliproxyPluginFree(void*, size_t);
extern void cliproxyPluginShutdown(void);

static const cliproxy_host_api* stored_host;
static void store_host_api(const cliproxy_host_api* host) { stored_host = host; }
static int call_host_api(const char* method, const uint8_t* request, size_t len, cliproxy_buffer* response) {
	if (stored_host == NULL || stored_host->call == NULL) return 1;
	return stored_host->call(stored_host->host_ctx, method, request, len, response);
}
static void free_host_buffer(void* ptr, size_t len) {
	if (stored_host != NULL && stored_host->free_buffer != NULL && ptr != NULL) stored_host->free_buffer(ptr, len);
}
*/
import "C"

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"unsafe"

	"gopkg.in/yaml.v3"
)

const (
	pluginID      = "claude-deepseek-search"
	defaultSchema = uint32(3)
)

var pluginVersion = "0.2.0-dev"

const (
	methodPluginRegister        = "plugin.register"
	methodPluginReconfigure     = "plugin.reconfigure"
	methodModelRoute            = "model.route"
	methodExecutorIdentifier    = "executor.identifier"
	methodExecutorExecute       = "executor.execute"
	methodExecutorExecuteStream = "executor.execute_stream"
	methodExecutorCountTokens   = "executor.count_tokens"
	methodHostHTTPDo            = "host.http.do"
	methodHostHTTPOperationOpen = "host.http.operation_open"
	methodHostHTTPCancel        = "host.http.cancel"
	methodHostStreamEmit        = "host.stream.emit"
	methodHostStreamClose       = "host.stream.close"
)

type envelope struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	HTTPStatus int    `json:"http_status,omitempty"`
}

type lifecycleRequest struct {
	ConfigYAML    []byte `json:"config_yaml"`
	SchemaVersion uint32 `json:"schema_version"`
}

type routeRequest struct {
	SourceFormat   string
	RequestedModel string
	Body           []byte
	Metadata       map[string]any
}

type executorRequest struct {
	Model           string
	Format          string
	Stream          bool
	OriginalRequest []byte
	Payload         []byte
	Metadata        map[string]any
	HostCallbackID  string
	StreamID        string
}

type executorResponse struct {
	Payload []byte      `json:"Payload"`
	Headers http.Header `json:"Headers"`
}

type modelRouteResponse struct {
	Handled    bool   `json:"Handled"`
	TargetKind string `json:"TargetKind,omitempty"`
	Target     string `json:"Target,omitempty"`
	Reason     string `json:"Reason,omitempty"`
}

type pluginConfig struct {
	Enabled              bool           `yaml:"enabled"`
	APIKey               string         `yaml:"api_key"`
	APIKeyEnv            string         `yaml:"api_key_env"`
	BaseURL              string         `yaml:"base_url"`
	Models               []string       `yaml:"models"`
	ExcludeModels        []string       `yaml:"exclude_models"`
	RequireWebSearchOnly bool           `yaml:"require_web_search_only"`
	Count                int            `yaml:"count"`
	NeedContent          bool           `yaml:"need_content"`
	TimeoutSeconds       int            `yaml:"timeout_seconds"`
	Priority             int            `yaml:"priority"` // CPA-owned; accepted but not used by the plugin.
	Store                map[string]any `yaml:"store"`    // CPA-owned plugin-store metadata.
}

var activeConfig atomic.Pointer[pluginConfig]
var activeSchema atomic.Uint32

func main() {}

//export cliproxy_plugin_init
func cliproxy_plugin_init(host *C.cliproxy_host_api, plugin *C.cliproxy_plugin_api) C.int {
	if host == nil || plugin == nil || host.abi_version != 1 {
		return 1
	}
	C.store_host_api(host)
	plugin.abi_version = 1
	plugin.call = C.cliproxy_plugin_call_fn(C.cliproxyPluginCall)
	plugin.free_buffer = C.cliproxy_plugin_free_fn(C.cliproxyPluginFree)
	plugin.shutdown = C.cliproxy_plugin_shutdown_fn(C.cliproxyPluginShutdown)
	return 0
}

//export cliproxyPluginCall
func cliproxyPluginCall(method *C.char, request *C.uint8_t, requestLen C.size_t, response *C.cliproxy_buffer) C.int {
	if response == nil || method == nil {
		return 1
	}
	response.ptr = nil
	response.len = 0
	var rawRequest []byte
	if request != nil && requestLen > 0 {
		rawRequest = C.GoBytes(unsafe.Pointer(request), C.int(requestLen))
	}
	raw, errCall := handleMethod(C.GoString(method), rawRequest)
	if errCall != nil {
		raw = errorEnvelope("plugin_error", errCall.Error(), 0)
	}
	writeResponse(response, raw)
	if errCall != nil {
		return 1
	}
	return 0
}

//export cliproxyPluginFree
func cliproxyPluginFree(ptr unsafe.Pointer, _ C.size_t) {
	if ptr != nil {
		C.free(ptr)
	}
}

//export cliproxyPluginShutdown
func cliproxyPluginShutdown() {}

func handleMethod(method string, raw []byte) ([]byte, error) {
	switch method {
	case methodPluginRegister, methodPluginReconfigure:
		if err := configure(raw); err != nil {
			return nil, err
		}
		return okEnvelope(pluginRegistration())
	case methodModelRoute:
		return routeModel(raw)
	case methodExecutorIdentifier:
		return okEnvelope(map[string]string{"identifier": pluginID})
	case methodExecutorExecute:
		return execute(raw)
	case methodExecutorExecuteStream:
		return executeStream(raw)
	case methodExecutorCountTokens:
		return okEnvelope(map[string]any{"Payload": []byte(`{"input_tokens":0}`)})
	default:
		return errorEnvelope("unknown_method", "unknown method", 404), nil
	}
}

func configure(raw []byte) error {
	var request lifecycleRequest
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &request); err != nil {
			return fmt.Errorf("decode plugin configuration: %w", err)
		}
	}
	cfg := defaultConfig()
	if len(request.ConfigYAML) > 0 {
		decoder := yaml.NewDecoder(strings.NewReader(string(request.ConfigYAML)))
		decoder.KnownFields(true)
		if err := decoder.Decode(&cfg); err != nil {
			return fmt.Errorf("decode plugin YAML: %w", err)
		}
	}
	normalizeConfig(&cfg)
	if cfg.Enabled && len(cfg.Models) == 0 {
		return errors.New("enabled plugin requires a non-empty models allowlist")
	}
	if cfg.Enabled && resolveAPIKey(cfg) == "" {
		return errors.New("DeepSeek API key is required when the plugin is enabled")
	}
	activeConfig.Store(&cfg)
	activeSchema.Store(negotiateSchema(request.SchemaVersion))
	return nil
}

func negotiateSchema(hostSchema uint32) uint32 {
	if hostSchema == 0 {
		return defaultSchema
	}
	if hostSchema > 6 {
		return 6
	}
	return hostSchema
}

func defaultConfig() pluginConfig {
	return pluginConfig{
		APIKeyEnv:            "DEEPSEEK",
		BaseURL:              "https://api.deepseek.com",
		RequireWebSearchOnly: true,
		Count:                5,
		TimeoutSeconds:       30,
	}
}

func normalizeConfig(cfg *pluginConfig) {
	cfg.APIKey = strings.TrimSpace(cfg.APIKey)
	cfg.APIKeyEnv = strings.TrimSpace(cfg.APIKeyEnv)
	if cfg.APIKeyEnv == "" {
		cfg.APIKeyEnv = "DEEPSEEK"
	}
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.deepseek.com"
	}
	if cfg.Count < 1 || cfg.Count > 50 {
		cfg.Count = 5
	}
	if cfg.TimeoutSeconds <= 0 || cfg.TimeoutSeconds > 300 {
		cfg.TimeoutSeconds = 30
	}
	cfg.Models = trimPatterns(cfg.Models)
	cfg.ExcludeModels = trimPatterns(cfg.ExcludeModels)
}

func trimPatterns(patterns []string) []string {
	out := make([]string, 0, len(patterns))
	for _, pattern := range patterns {
		if pattern = strings.TrimSpace(pattern); pattern != "" {
			out = append(out, pattern)
		}
	}
	return out
}

func resolveAPIKey(cfg pluginConfig) string {
	if cfg.APIKey != "" {
		return cfg.APIKey
	}
	return strings.TrimSpace(os.Getenv(cfg.APIKeyEnv))
}

func loadedConfig() pluginConfig {
	if cfg := activeConfig.Load(); cfg != nil {
		return *cfg
	}
	return pluginConfig{}
}

func pluginRegistration() map[string]any {
	return map[string]any{
		"schema_version": activeSchema.Load(),
		"metadata": map[string]any{
			"Name":             pluginID,
			"Version":          pluginVersion,
			"Author":           "SyntaxSmith",
			"GitHubRepository": "https://github.com/SyntaxSmith/claude-deepseek-search",
			"ConfigFields": []any{
				map[string]any{"Name": "enabled", "Type": "boolean", "Description": "Enable the adapter."},
				map[string]any{"Name": "models", "Type": "array", "Description": "Required case-insensitive allowlist; supports * and ? wildcards."},
				map[string]any{"Name": "exclude_models", "Type": "array", "Description": "Optional denylist checked after models."},
				map[string]any{"Name": "api_key", "Type": "string", "Description": "DeepSeek API key; prefer api_key_env for secrets."},
				map[string]any{"Name": "api_key_env", "Type": "string", "Description": "Environment variable containing the DeepSeek API key."},
				map[string]any{"Name": "count", "Type": "integer", "Description": "Search results per request, from 1 to 50."},
				map[string]any{"Name": "need_content", "Type": "boolean", "Description": "Ask DeepSeek Search to include page content."},
				map[string]any{"Name": "timeout_seconds", "Type": "integer", "Description": "DeepSeek request deadline, from 1 to 300 seconds."},
			},
		},
		"capabilities": map[string]any{
			"model_router":            true,
			"executor":                true,
			"executor_model_scope":    "static",
			"executor_input_formats":  []string{"claude"},
			"executor_output_formats": []string{"claude"},
		},
	}
}

func routeModel(raw []byte) ([]byte, error) {
	var req routeRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, fmt.Errorf("decode model route request: %w", err)
	}
	cfg := loadedConfig()
	if !cfg.Enabled || !isClaudeSourceFormat(req.SourceFormat) {
		return okEnvelope(modelRouteResponse{Handled: false})
	}
	if !modelAllowed(req.RequestedModel, cfg) {
		return okEnvelope(modelRouteResponse{Handled: false, Reason: "model_not_in_allowlist"})
	}
	if !isClaudeCodeBuiltinWebSearchRequest(req.Body, cfg.RequireWebSearchOnly) &&
		!isDirectWebFetchRequest(req.Body, cfg.RequireWebSearchOnly) {
		return okEnvelope(modelRouteResponse{Handled: false, Reason: "not_a_claude_code_web_request"})
	}
	return okEnvelope(modelRouteResponse{
		Handled:    true,
		TargetKind: "self",
		Target:     pluginID,
		Reason:     "allowlisted_model_deepseek_search",
	})
}

func execute(raw []byte) ([]byte, error) {
	var req executorRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, fmt.Errorf("decode executor request: %w", err)
	}
	body, errSearch := executeWebRequest(req)
	if errSearch != nil {
		return errorEnvelope("executor_error", errSearch.Error(), errHTTPStatus(errSearch)), nil
	}
	return okEnvelope(executorResponse{Payload: body, Headers: http.Header{"Content-Type": []string{"application/json"}}})
}

func executeStream(raw []byte) ([]byte, error) {
	var req executorRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, fmt.Errorf("decode stream executor request: %w", err)
	}
	if strings.TrimSpace(req.StreamID) == "" {
		return errorEnvelope("executor_error", "stream_id is required", 400), nil
	}
	payload, errSearch := executeWebRequest(req)
	if errSearch != nil {
		return errorEnvelope("executor_error", errSearch.Error(), errHTTPStatus(errSearch)), nil
	}
	go func() {
		if _, errEmit := callHost(methodHostStreamEmit, map[string]any{"stream_id": req.StreamID, "payload": payload}); errEmit != nil {
			closePluginStream(req.StreamID, errEmit.Error())
			return
		}
		closePluginStream(req.StreamID, "")
	}()
	return okEnvelope(map[string]any{"Headers": http.Header{"Content-Type": []string{"text/event-stream"}}})
}

func executeSearch(req executorRequest) ([]byte, error) {
	cfg := loadedConfig()
	query := extractClaudeWebSearchQuery(req.OriginalRequest)
	if query == "" {
		query = extractClaudeWebSearchQuery(req.Payload)
	}
	if query == "" {
		return nil, newHTTPError(400, "web search query is empty")
	}
	client := &deepSeekClient{
		APIKey:         resolveAPIKey(cfg),
		BaseURL:        cfg.BaseURL,
		Count:          cfg.Count,
		NeedContent:    cfg.NeedContent,
		TimeoutSeconds: cfg.TimeoutSeconds,
	}
	hits, errSearch := client.search(query, cfg.Count, req.HostCallbackID)
	if errSearch != nil {
		return nil, errSearch
	}
	builder := newClaudeResponseBuilder(req.Model)
	if req.Stream {
		return builder.buildStream(query, hits), nil
	}
	return builder.buildMessage(query, hits), nil
}

func okEnvelope(result any) ([]byte, error) {
	payload, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	return json.Marshal(envelope{OK: true, Result: payload})
}

func errorEnvelope(code, message string, status int) []byte {
	payload, _ := json.Marshal(envelope{OK: false, Error: &rpcError{Code: code, Message: message, HTTPStatus: status}})
	return payload
}

func writeResponse(response *C.cliproxy_buffer, raw []byte) {
	if response == nil || len(raw) == 0 {
		return
	}
	ptr := C.CBytes(raw)
	if ptr == nil {
		return
	}
	response.ptr = ptr
	response.len = C.size_t(len(raw))
}

func callHost(method string, payload any) (json.RawMessage, error) {
	rawPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal host callback: %w", err)
	}
	cMethod := C.CString(method)
	defer C.free(unsafe.Pointer(cMethod))
	var response C.cliproxy_buffer
	var requestPtr *C.uint8_t
	if len(rawPayload) > 0 {
		buf := C.CBytes(rawPayload)
		if buf == nil {
			return nil, errors.New("allocate host callback request")
		}
		defer C.free(buf)
		requestPtr = (*C.uint8_t)(buf)
	}
	code := C.call_host_api(cMethod, requestPtr, C.size_t(len(rawPayload)), &response)
	var rawResponse []byte
	if response.ptr != nil && response.len > 0 {
		rawResponse = C.GoBytes(response.ptr, C.int(response.len))
		C.free_host_buffer(response.ptr, response.len)
	}
	if len(rawResponse) == 0 {
		return nil, fmt.Errorf("host callback %s returned no response, code=%d", method, int(code))
	}
	var env envelope
	if err := json.Unmarshal(rawResponse, &env); err != nil {
		return nil, fmt.Errorf("decode host callback envelope: %w", err)
	}
	if !env.OK {
		if env.Error != nil {
			return nil, newHTTPError(env.Error.HTTPStatus, env.Error.Message)
		}
		return nil, fmt.Errorf("host callback %s failed", method)
	}
	if code != 0 {
		return nil, fmt.Errorf("host callback %s returned code=%d", method, int(code))
	}
	return env.Result, nil
}

func closePluginStream(streamID, message string) {
	_, _ = callHost(methodHostStreamClose, map[string]string{"stream_id": streamID, "error": message})
}
