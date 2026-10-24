# Claude DeepSeek Search

CLIProxyAPI v8 插件：让 Claude Code 的 WebSearch 请求使用 DeepSeek Search，适合 GLM、Qwen、Kimi 等没有原生 WebSearch 的模型。

## 特性

- `models` allowlist：只接管明确配置的模型，支持 `*` 和 `?`。
- GPT、Claude、DeepSeek 模型保留原生搜索路径。
- 支持 Claude WebSearch 的非流式和 SSE 响应。
- DeepSeek API key 只在 CPA 私有配置中使用，不进入项目仓库。
- 当前只处理 WebSearch，不处理 WebFetch。
- `require_web_search_only` 默认只接管单独的 WebSearch 请求。

带域名过滤、地域定位或动态 caller 的请求会交给其他路径处理。

## 构建

需要 Go 1.26+ 和 macOS Apple Silicon 的 C 编译器：

```bash
make build
make test
```

产物是 `build/claude-deepseek-search.dylib`。项目不提交构建产物。

## 安装

CPA 使用 Homebrew 时，插件目录通常为 `~/.cli-proxy-api/plugins/darwin/arm64/`：

```bash
cp build/claude-deepseek-search.dylib \
  ~/.cli-proxy-api/plugins/darwin/arm64/
```

在 CPA 的 `plugins.configs` 中加入：

```yaml
claude-deepseek-search:
  enabled: true
  priority: 20
  models:
    - "GLM-*"
  api_key_env: DEEPSEEK
  base_url: https://api.deepseek.com
  count: 5
  timeout_seconds: 30
```

也可以使用本地私有 `api_key`，不要把它写进 Git。CPA 是 launchd 服务，确保 key 对服务进程可见。安装后重启现有 CPA 服务即可。

## 测试

```bash
make test
(cd go && go test -race ./... && go vet ./...)
```

单元测试覆盖请求检测、响应转换和错误脱敏；CPA host ABI 需在目标环境中验证。

## 许可证

MIT，见 `LICENSE` 和 `NOTICE.md`。
