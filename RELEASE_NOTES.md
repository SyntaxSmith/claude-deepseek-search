## v0.2.0

### Features

- Allowlisted Claude Code WebSearch requests use DeepSeek Search without MCP.
- Direct Messages API `web_fetch_*` requests can use DeepSeek Fetch, with JSON and SSE document responses.
- Shared model allowlist, CPA HTTP cancellation/timeout, and API-key redaction.

### Fixes

- Streaming Fetch uses the same guarded dispatcher as non-streaming requests.
- Mixed Search/Fetch requests no longer bypass Fetch option checks.
- Fetch uses the current user turn instead of historical URLs, and preserves parentheses in URLs.

### Distribution

- Prebuilt packages: Linux amd64/arm64, macOS amd64/arm64, Windows amd64.
- Each package includes the library, configuration example, README, license and upstream notices.
- `checksums.txt` contains SHA-256 hashes. Linux binaries target glibc systems (built on Ubuntu 22.04); Alpine requires a source build. macOS binaries target macOS 12+ and are not Developer ID signed/notarized.
- Builds are tested and the shared-library ABI exports are loaded on their native runners.

### Scope

Requires CPA v8 with dynamic plugins and cancellable host HTTP operations (verified locally on 8.0.40). **This release does not replace Claude Code's client-side WebFetch.** Fetch support is for explicitly forced, single-URL Messages API Fetch requests; it is not a general server-tool/model loop. No MCP server is added.
