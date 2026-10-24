# Claude DeepSeek Search

为 [CLIProxyAPI（CPA）](https://github.com/router-for-me/CLIProxyAPI) 补充 DeepSeek 搜索和网页抓取能力。按模型名单启用，不需要安装 MCP。

[下载 Release](https://github.com/SyntaxSmith/claude-deepseek-search/releases/latest) · [配置示例](config.example.yaml) · [发行说明](RELEASE_NOTES.md)

## 能做什么

| 入口 | 当前支持 |
| --- | --- |
| Claude Code 原生 `WebSearch` | 识别独立搜索请求，由 DeepSeek Search 返回结果 |
| Messages API `web_fetch_*` | 对显式指定 `web_fetch` 的单 URL 请求，调用 DeepSeek Fetch，返回 Markdown 文档 |
| Claude Code 原生 `WebFetch` | **不替换**。它由客户端抓取网页，并非上述服务端 Fetch API |

只处理 `models` 中的模型，例如 `GLM-*`；GPT、Claude、DeepSeek 模型保持原有路径。普通模型对话不受影响。Search 和 API Fetch 均支持 JSON、SSE 响应。

推荐搜索一次返回 **5 条**，需要交叉核验时可设为 8–10 条。搜索返回摘要，抓取返回正文。

## 运行要求

- 支持动态插件与 `host.http.operation_open` 的 CPA v8 构建；不适用于 `no-plugin` 构建。已在 CPA 8.0.40 验证。
- DeepSeek **官方 API key**，供 `https://api.deepseek.com/v1/tools/search` 和 `/v1/tools/fetch` 使用。
- 插件的系统、CPU 架构与 **CPA 进程**一致。Docker 用户应选容器平台，而不是宿主机平台。

## 1. 下载对应安装包

从 [Releases](https://github.com/SyntaxSmith/claude-deepseek-search/releases) 下载发行包和 `checksums.txt`。v0.2.0 提供：

| 平台 | 安装包 |
| --- | --- |
| Linux x86-64 | `claude-deepseek-search_0.2.0_linux_amd64.tar.gz` |
| Linux ARM64 | `claude-deepseek-search_0.2.0_linux_arm64.tar.gz` |
| macOS Intel | `claude-deepseek-search_0.2.0_darwin_amd64.tar.gz` |
| macOS Apple Silicon | `claude-deepseek-search_0.2.0_darwin_arm64.tar.gz` |
| Windows x64 | `claude-deepseek-search_0.2.0_windows_amd64.tar.gz` |

Linux 包使用 glibc（Ubuntu 22.04 构建），Alpine/musl 请在目标环境从源码构建。macOS 包面向 macOS 12+，未做 Developer ID 签名或公证。Windows ARM64 暂无预编译包。

校验下载文件：

```bash
# Linux
sha256sum --ignore-missing -c checksums.txt
# macOS（与 checksums.txt 中对应的哈希比较）
shasum -a 256 claude-deepseek-search_0.2.0_darwin_arm64.tar.gz
```

Windows 可使用 PowerShell 的 `Get-FileHash <安装包> -Algorithm SHA256`。

## 2. 放进 CPA 插件目录

安装位置由 CPA 配置的 **`plugins.dir`** 决定，与 Homebrew、systemd 或其他安装方式无关。保留现有插件目录，不要覆盖整个配置。

目标目录结构：

```text
<plugins.dir>/
├── linux/amd64/claude-deepseek-search.so
├── darwin/arm64/claude-deepseek-search.dylib
└── windows/amd64/claude-deepseek-search.dll
```

只需放置当前平台的文件。例如 Linux x86-64：

```bash
tar -xzf claude-deepseek-search_0.2.0_linux_amd64.tar.gz
mkdir -p /path/to/plugins/linux/amd64
cp claude-deepseek-search_0.2.0_linux_amd64/linux/amd64/claude-deepseek-search.so \
   /path/to/plugins/linux/amd64/
```

Windows 自带的 `tar -xzf` 也可解包，然后复制 `windows/amd64/` 中的 DLL。Docker 部署则将插件目录挂载进容器，并让 `plugins.dir` 指向**容器内**的路径。

## 3. 合并配置并重启

把以下内容合并进 CPA 配置，保留已有的 `plugins.configs` 条目：

```yaml
plugins:
  enabled: true
  dir: /path/to/plugins
  configs:
    claude-deepseek-search:
      enabled: true
      priority: 20
      models:
        - "GLM-*"
      api_key_env: DEEPSEEK
      count: 5
      timeout_seconds: 30
```

设置 CPA **进程环境**中的 `DEEPSEEK`。手动启动示例：

```bash
export DEEPSEEK='你的官方 API key'
cliproxyapi -config /path/to/config.yaml
```

也可以在私有 CPA 配置中填写 `api_key`，它优先于环境变量。不要把真实 key 上传到仓库。后台服务和容器需要通过其环境配置传入 key，不会自动继承当前终端的 `export`。

使用原有的 CPA 管理方式重启：systemd 重启对应 unit、Docker 重启对应容器、Homebrew 用户执行 `brew services restart cliproxyapi`，手动启动则重启进程。

确认启动日志出现 `plugin registered` 和 `plugin_id=claude-deepseek-search`，然后在 Claude Code 中切换到名单内的模型，使用原生 WebSearch。若提示词规则强制使用其他搜索工具，需要相应调整；安装插件本身不会改写这些规则。

### 可选配置

| 字段 | 默认值 / 用途 |
| --- | --- |
| `models` | 空列表；填写需要接管的模型，支持大小写不敏感的 `*`、`?` |
| `exclude_models` | `[]`，排除项优先 |
| `api_key_env` | `DEEPSEEK` |
| `base_url` | `https://api.deepseek.com` |
| `count` | `5`，Search 结果数，范围 1–50 |
| `need_content` | `false`，向 Search 请求网页正文 |
| `timeout_seconds` | `30`，请求超时 |
| `require_web_search_only` | `true`，只处理独立的 Search 或 Fetch 工具请求 |

## API Fetch 的用法

下面是可被插件接管的 Messages 请求。它返回抓取文档，不会另外调用 GLM 生成总结：

```json
{
  "model": "GLM-5.3",
  "max_tokens": 1024,
  "tools": [{"type": "web_fetch_20250910", "name": "web_fetch"}],
  "tool_choice": {"type": "tool", "name": "web_fetch"},
  "messages": [{"role": "user", "content": "Fetch https://example.com/article"}]
}
```

较新的 `web_search_*` / `web_fetch_*` 版本需要显式设置 `allowed_callers: ["direct"]`。域名限制、地域定位、动态过滤及 citations、`max_content_tokens` 等高级 Fetch 选项不在当前适配范围内，带这些选项的请求保留给原有上游。搜索结果中的 `encrypted_content` 是本插件的 Base64 JSON，不是 Anthropic 原生的加密重放数据。

## 从源码构建

需要 Go 1.26+、GNU Make 和 C 编译器。Linux 使用 GCC，macOS 使用 Xcode Command Line Tools，Windows 可使用 MSYS2 UCRT64 + GCC。

```bash
git clone https://github.com/SyntaxSmith/claude-deepseek-search.git
cd claude-deepseek-search
make test
make build
make package VERSION=0.2.0
```

默认构建当前平台，库文件在 `build/`，安装包在 `dist/`。跨平台构建需要对应的 C 交叉编译工具链，仅设置 `GOOS`/`GOARCH` 不够。

发行构建位于 [GitHub Actions](.github/workflows/release.yml)：各平台独立运行测试、编译和动态库加载检查；全部通过后发布安装包与 SHA-256 校验文件。

## 许可证与致谢

MIT。参考了 [karuboniru/deepseek-search](https://github.com/karuboniru/deepseek-search) 和 CPA 的 `claude-web-search-router` 示例，详见 [NOTICE.md](NOTICE.md)。
