## v0.2.1

### Fix

- Allowlisted DeepSeek models are intercepted again. `deepseek*` was pinned to the provider-native path, but DeepSeek's chat API has no server-side web search, so those requests skipped the adapter and were answered by an upstream without search. The allowlist now decides for DeepSeek; `gpt*`, `o1*`–`o5*` and `claude*` keep their native path.

### Upgrade

- Drop-in: replace the shared library, then restart CPA. No configuration change is required — an existing `models` allowlist that lists DeepSeek models starts working without edits.
