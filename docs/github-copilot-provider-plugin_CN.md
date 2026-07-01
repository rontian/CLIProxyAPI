# GitHub Copilot Provider 插件方案

本文说明 `github-copilot` provider 的边界、认证流程、调用方式和部署方法。

## 定位

`github-copilot` 不作为 CLIProxyAPI 核心硬编码 provider 接入，而是作为动态插件接入。源码维护在 `plugins-src/github-copilot/go`，编译产物部署到运行时 `plugins/` 目录。

- 插件能力：`AuthProvider`、`ModelProvider`、`ProviderExecutor`。
- provider key：`github-copilot`。
- 支持入口：优先支持 OpenAI Chat Completions 形态。
- 请求网络：通过插件宿主的 `host.http.do` / `host.http.do_stream` 发送，复用 CLIProxyAPI 的 `proxy-url`、请求日志和传输策略。

GitHub 官方 REST Copilot API 更偏管理、席位和用量统计。模型调用通常来自 Copilot 客户端内部协议，因此该插件把它隔离在插件层，避免把非公开协议散落进核心网关代码。

## 协议分层

CLIProxyAPI 的端点兼容不是每个 provider 独立复制一份。整体链路是：

```text
客户端端点
  -> CLIProxyAPI 入口协议处理
  -> 请求格式归一化/转换
  -> provider/auth 选择
  -> ProviderExecutor
  -> 上游服务
```

GitHub Copilot 插件第一版声明：

```text
ExecutorInputFormats:  chat-completions
ExecutorOutputFormats: chat-completions
```

所以最稳定的调用方式是 `/v1/chat/completions`。如果其他入口协议能被 CLIProxyAPI 现有转换层转成 `chat-completions`，也可以间接使用；如果未来需要原生支持 Responses，可在插件内继续声明 `responses` 或增加插件级 translator。

## 认证流程

第一版使用 GitHub OAuth device flow：

1. 插件向 GitHub 请求 device code。
2. 用户打开返回的 GitHub 登录 URL 完成授权。
3. 插件轮询 access token。
4. 插件保存 provider-owned auth JSON 到 CLIProxyAPI auth 目录。
5. 调用模型前，用 GitHub token 请求 Copilot short-lived token。
6. Copilot token 过期前由插件刷新。

登录成功只表示 GitHub OAuth 成功。账号是否具备 Copilot 订阅，以获取 Copilot token 或实际模型调用结果为准。

## 构建

macOS：

```bash
go build -buildmode=c-shared -o plugins/github-copilot.dylib ./plugins-src/github-copilot/go
```

Linux：

```bash
go build -buildmode=c-shared -o plugins/github-copilot.so ./plugins-src/github-copilot/go
```

插件文件名会成为插件 ID，所以建议固定为 `github-copilot.dylib` 或 `github-copilot.so`。

## 配置

在 `config.yaml` 中启用插件系统和插件实例：

```yaml
plugins:
  enabled: true
  dir: "plugins"
  configs:
    github-copilot:
      enabled: true
      priority: 10
      client-id: "Iv1.b507a08c87ecfe98"
      github-base-url: "https://github.com"
      github-api-base-url: "https://api.github.com"
      copilot-api-base-url: "https://api.githubcopilot.com"
      editor-version: "vscode/1.104.0"
      editor-plugin-version: "copilot-chat/0.30.0"
      user-agent: "GitHubCopilotChat/0.30.0"
      models:
        - "gpt-4.1"
        - "gpt-4o"
        - "claude-sonnet-4"
```

`models` 是模型发现失败时的兜底列表。实际可用模型仍以 GitHub Copilot token 和模型列表接口返回为准。

## 调用

登录完成后，模型列表会出现 `github-copilot` provider 暴露的模型。客户端可按 CLIProxyAPI 现有模型选择规则调用，例如：

```json
{
  "model": "gpt-4.1",
  "messages": [
    {"role": "user", "content": "hello"}
  ]
}
```

如果存在同名模型冲突，建议给该插件认证或模型使用前缀策略，或在客户端侧选择明确的 provider/model 组合。

## 当前限制

- 第一版只把 Copilot 上游调用包装成 Chat Completions provider。
- Copilot 的内部模型 API 不是稳定公开 API，后续可能需要随 GitHub 客户端协议调整插件。
- 订阅权限、组织策略、地区限制和网络代理问题只能通过 Copilot token 获取或实际调用结果验证。
- 不建议把该 provider 作为生产多租户共享服务公开给不可信用户使用。

## 后续方向

- 增加模型发现结果缓存和更清晰的订阅错误分类。
- 增加 Responses 格式支持。
- 在 CPA-Manager 中把该插件显示为独立 provider 登录入口。
- 为 Auto Router 增加 `github-copilot` 推荐模型和成本/能力分级。
