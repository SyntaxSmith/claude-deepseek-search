# Claude DeepSeek Search

CLIProxyAPI C ABI 插件：为 Claude Code 的内置 `web_search` 请求提供 DeepSeek Search 后端，主要用于 GLM、Qwen、Kimi 等没有可用原生 WebSearch 的模型。

## 设计边界

插件不是全局搜索代理：

1. 只有 `models` allowlist 命中的模型才会路由到插件；默认 allowlist 为空。
2. `exclude_models` 优先级高于 allowlist。
3. `gpt*`、`o1*`、`o3*`、`o4*`、`o5*`、`claude*`、`deepseek*` 始终视为原生搜索模型，即使 allowlist 写成 `*` 也不会被接管。
4. 只匹配 Claude / Anthropic Messages 协议、Claude Code 的 helper 提示，并默认要求工具列表仅包含 typed `web_search`；普通 Messages 请求即使声明了 WebSearch 也不会被接管。
5. 支持 `web_search_20250305`、`web_search_20260209` 和 `web_search_20260318` 的基础请求；`tool_choice: {"type":"none"}`、域名过滤、`user_location`、动态 caller 或 `response_inclusion: excluded` 请求会主动放弃路由，不会静默丢失语义。
6. 当前版本只接管 WebSearch；WebFetch 仍由 CPA/上游或 AnySearch 处理。先保证 Search 的协议兼容，再单独扩展 Fetch，避免伪造错误的 `web_fetch_tool_result` 格式。

`models` 支持大小写不敏感的 `*` 和 `?` 通配符，例如 `GLM-*` 会匹配 `GLM-5.3` 和 `GLM-5.3[1m]`。

## 构建

需要 Go 1.26+ 和 macOS Apple Silicon 的 C 编译器：

```bash
make build
make test
```

产物：`build/claude-deepseek-search.dylib`。

如果 CPA 由 Homebrew 管理，插件目录通常是 `~/.cli-proxy-api/plugins/darwin/arm64/`；本项目不会把构建产物提交到 GitHub，安装时由本机自行编译。完整的真实 CPA 假 Search API 集成检查可以运行：

```bash
python3 -I scripts/test-host.py \\
  --cpa /opt/homebrew/opt/cliproxyapi/bin/cliproxyapi
```

## 配置

复制 `config.example.yaml` 的 `plugins` 段，按实际路径和需要补 Search 的模型修改。CPA 会把宿主字段 `priority` 和插件商店的 `store` 元数据一并传给插件，代码已显式接受；插件注册时还会根据宿主 lifecycle 请求协商 schema。搜索执行需要 CPA v8 的 cancellable `host.http.operation_open` 接口。

不要把 DeepSeek key 提交到仓库；CPA 是 launchd 服务，终端里临时 `export DEEPSEEK=...` 不一定会传给它。推荐把 key 放进 CPA 的私有环境配置或 launchd 环境中；也可在未纳入版本控制的本地配置里设置 `api_key`。

启用前应至少先做静态检查：

```bash
# 假设当前 CPA 配置仍使用 plugins.dir
cp build/claude-deepseek-search.dylib \
  ~/.cli-proxy-api/plugins/darwin/arm64/claude-deepseek-search.dylib
```

CPA v8 使用 `plugins.dir` 发现动态库；可直接把 `dir` 指向本项目的 `build` 目录，或把 dylib 复制到现有的 `~/.cli-proxy-api/plugins/darwin/arm64/`。当前 v7 版本可以完成 ABI 注册协商，但不具备带 deadline 的 HTTP operation，因此不作为本插件的运行目标。两个插件不要给同一个模型同时路由。

`web_search_result` 中包含用于 Claude Code 重放的 `encrypted_content` 兼容字段；该字段是给非 Anthropic 上游使用的版本化 Base64 上下文封装，不是真正的加密证明，因此本插件的结果不应再转发到 Anthropic 原生 WebSearch API。

## 测试策略

测试覆盖：

- allowlist 未命中时不接管；
- GPT、Claude、DeepSeek 原生模型即使使用 `*` 也不接管；
- GLM 命中且请求是 Claude Code WebSearch 时接管；
- 混合工具请求在默认 `require_web_search_only: true` 下不接管；
- DeepSeek Search 响应转换为 Claude 非流式消息和 SSE 消息。

本地测试使用伪造的 host callback，不访问 DeepSeek，也不需要 API key。
