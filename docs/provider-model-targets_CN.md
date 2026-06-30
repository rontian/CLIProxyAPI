# CLIProxyAPI 提供商、后端类型与 Auto Router 目标关系

本文说明 CPA-Manager 里的“AI 提供商配置”、OAuth 认证、OpenAI 兼容配置，以及 Auto Router 中 `provider/model` 的关系。

## 核心结论

CLIProxyAPI 对外暴露的是模型名，例如 `auto`、`gpt-5-codex`、`gemini-2.5-flash`、`deepseek-chat`。但在内部执行请求时，还需要知道这个模型应该走哪条后端执行链路。Auto Router 的 `provider` 字段就是这个内部后端类型，也可以理解为“路由目标通道”。

因此：

- 客户端仍然只请求 CLIProxyAPI 暴露的模型，例如 `model: "auto"`。
- Auto Router 选中角色后，会把请求改派到某个内部目标：`provider + model`。
- 一个 provider 可以提供多个模型。`provider` 选择后端组，`model` 选择该后端组里的具体模型或别名。
- `provider` 不等于客户端请求路径，也不表示客户端绕过 CLIProxyAPI。
- OpenAI 兼容提供商不是写成 `openai/deepseek`。如果配置名是 `deepseek`，内部 provider key 通常是 `openai-compatible-deepseek`。

## 名词对照

| 名词 | 所在位置 | 含义 | 示例 |
| --- | --- | --- | --- |
| AI 提供商配置 | CPA-Manager 的提供商配置页 | 用户管理密钥、OAuth 账号、OpenAI 兼容端点的地方 | Gemini API 密钥、Codex API 配置、Claude API 配置、OpenAI 兼容提供商 |
| OAuth 认证 | `auth-dir` 下的认证文件或管理端 OAuth 流程 | 文件型账号凭据，通常对应内置渠道 | `type: codex`、`type: claude`、`type: antigravity` |
| OpenAI 兼容提供商 | `openai-compatibility` 配置数组 | 任意兼容 OpenAI API 的第三方端点 | `name: deepseek`、`name: openrouter`、`name: siliconflow` |
| provider / 后端类型 | Auto Router 的 `provider` 字段、内部 executor key | CLIProxyAPI 内部用来选择执行器和凭据池的 key | `codex`、`claude`、`gemini`、`openai-compatible-deepseek` |
| model / 目标模型 | Auto Router 的 `model` 字段 | 选中后实际转发给该后端的模型名或别名 | `gpt-5-codex`、`claude-sonnet-4-5`、`deepseek-chat` |
| 模型别名 | 各类 `models[].alias` 或 `oauth-model-alias` | 客户端可见模型名到上游模型名的映射 | `deepseek-chat` -> `deepseek-reasoner` |

## 内置后端类型

这些 provider key 可以直接用在 Auto Router 的 `provider` 中：

| Auto Router `provider` | 来源配置或认证 | 说明 |
| --- | --- | --- |
| `gemini` | `gemini-api-key` | Gemini API Key 后端 |
| `codex` | `codex-api-key` 或 Codex OAuth auth 文件 | Codex 后端 |
| `claude` | `claude-api-key` 或 Claude OAuth auth 文件 | Claude 后端 |
| `vertex` | `vertex-api-key` 或 Vertex OAuth auth 文件 | Vertex 后端 |
| `aistudio` | OAuth auth 文件 | AI Studio / Gemini Web 类后端 |
| `antigravity` | OAuth auth 文件 | Antigravity 后端 |
| `kimi` | OAuth auth 文件 | Kimi 后端 |
| `xai` | OAuth auth 文件 | xAI 后端 |
| 插件 provider key | 插件配置或插件 OAuth | 插件自己声明的后端 |

示例：

```yaml
auto-router:
  enabled: true
  models:
    - name: "auto"
      fallback:
        provider: "claude"
        model: "claude-sonnet-4-5"
      roles:
        - id: "coding"
          provider: "codex"
          model: "gpt-5-codex"
```

这里客户端只请求 `auto`，CLIProxyAPI 内部可能把请求转给 `codex/gpt-5-codex` 或 `claude/claude-sonnet-4-5`。

## OpenAI 兼容提供商如何映射

OpenAI 兼容配置在 `config.yaml` 中是 `openai-compatibility` 数组：

```yaml
openai-compatibility:
  - name: "deepseek"
    base-url: "https://api.deepseek.com/v1"
    api-key-entries:
      - api-key: "sk-..."
    models:
      - name: "deepseek-chat"
        alias: "deepseek-chat"
      - name: "deepseek-reasoner"
        alias: "deepseek-reasoner"
```

这类配置有两层名字：

| 层级 | 值 | 用途 |
| --- | --- | --- |
| 配置组名 | `deepseek` | CPA-Manager 展示、凭据归属、生成内部 provider key |
| 内部 provider key | `openai-compatible-deepseek` | Auto Router 和执行器选择时使用 |
| 模型名或别名 | `deepseek-chat` | Auto Router 的 `model` 字段和客户端可见模型 |

同一个 OpenAI 兼容提供商可以配置多个模型，Auto Router 通过同一个 `provider` 搭配不同 `model` 来区分：

```yaml
openai-compatibility:
  - name: "deepseek"
    base-url: "https://api.deepseek.com/v1"
    api-key-entries:
      - api-key: "sk-..."
    models:
      - name: "deepseek-chat"
        alias: "ds-chat"
      - name: "deepseek-reasoner"
        alias: "ds-reasoner"

auto-router:
  enabled: true
  models:
    - name: "auto"
      fallback:
        provider: "openai-compatible-deepseek"
        model: "ds-chat"
      roles:
        - id: "fast"
          provider: "openai-compatible-deepseek"
          model: "ds-chat"
        - id: "reasoning"
          provider: "openai-compatible-deepseek"
          model: "ds-reasoner"
```

上面两个角色的 `provider` 一样，因为都走 DeepSeek 这个 OpenAI 兼容提供商；`model` 不一样，所以实际调用不同模型。

所以如果你配置了一个 OpenAI 兼容提供商 `name: "deepseek"`，Auto Router 推荐这样写：

```yaml
auto-router:
  enabled: true
  models:
    - name: "auto"
      fallback:
        provider: "openai-compatible-deepseek"
        model: "deepseek-chat"
      roles:
        - id: "reasoning"
          provider: "openai-compatible-deepseek"
          model: "deepseek-reasoner"
```

不要写成：

```yaml
provider: "openai/deepseek"
```

`openai/deepseek` 不是当前 CLIProxyAPI 的 provider key 规则。

如果 OpenAI 兼容配置没有 `name`，或你确实要使用通用兼容执行器，可以使用：

```yaml
provider: "openai-compatibility"
```

但有多个 OpenAI 兼容提供商时，更推荐使用带名称的内部 key，例如 `openai-compatible-deepseek`、`openai-compatible-openrouter`、`openai-compatible-siliconflow`，这样 Auto Router 可以稳定指向指定兼容提供商。

## OAuth 认证与 provider 的关系

OAuth 认证文件通常带有 `type` 字段，例如：

```json
{
  "type": "codex",
  "email": "user@example.com"
}
```

这个 `type` 基本就是该账号所属的内部 provider key。Auto Router 配置角色时，如果要使用这个 OAuth 账号池，对应写：

```yaml
provider: "codex"
model: "gpt-5-codex"
```

同理：

| OAuth `type` | Auto Router `provider` |
| --- | --- |
| `codex` | `codex` |
| `claude` | `claude` |
| `antigravity` | `antigravity` |
| `aistudio` | `aistudio` |
| `kimi` | `kimi` |
| `xai` | `xai` |
| 插件 OAuth provider | 插件 provider key |

OAuth 的模型别名通过 `oauth-model-alias` 或 auth 文件内的 `model-aliases` 生效。Auto Router 的 `model` 可以写客户端可见别名，但要注意别名不要跨 provider 重名，否则后端选择会变得不直观。

OAuth provider 也可能提供多个模型。例如多个 Codex 模型都属于 `provider: "codex"`：

```yaml
roles:
  - id: "coding-fast"
    provider: "codex"
    model: "gpt-5-codex-mini"
  - id: "coding-strong"
    provider: "codex"
    model: "gpt-5-codex"
```

这里不是两个 provider，而是同一个 Codex provider 下的两个模型目标。

## 模型别名与 Auto Router

Auto Router 的 `model` 字段建议写“客户端可见且在该 provider 下唯一”的模型名或别名。

例如 OpenAI 兼容配置：

```yaml
openai-compatibility:
  - name: "deepseek"
    models:
      - name: "deepseek-reasoner"
        alias: "ds-r1"
```

Auto Router 可以写：

```yaml
provider: "openai-compatible-deepseek"
model: "ds-r1"
```

如果你写上游原始模型名：

```yaml
model: "deepseek-reasoner"
```

也可以，但前提是该模型名确实被这个 provider 注册并可路由。管理端配置时，优先使用模型列表中能看到的模型名或别名。

## 推荐配置规则

1. 内置 API Key 或 OAuth 后端：`provider` 写内置 key，例如 `gemini`、`codex`、`claude`。
2. OpenAI 兼容后端：如果配置了 `name: deepseek`，`provider` 写 `openai-compatible-deepseek`。
3. `model` 写该 provider 下可见的模型名或别名。
4. 不要把 provider 和 model 拼在一起写成 `openai/deepseek`、`deepseek/deepseek-chat`。
5. 如果同一个模型别名在多个 provider 中重复，Auto Router 要显式配置不同 provider，避免路由歧义。
6. `fallback.provider/fallback.model` 只是兜底目标，不是客户端看到的新模型。

## 常见例子

### Codex 代码角色

```yaml
roles:
  - id: "coding"
    provider: "codex"
    model: "gpt-5-codex"
```

### Gemini 快速角色

```yaml
roles:
  - id: "fast"
    provider: "gemini"
    model: "gemini-2.5-flash"
```

### Claude 写作角色

```yaml
roles:
  - id: "writing"
    provider: "claude"
    model: "claude-sonnet-4-5"
```

### DeepSeek OpenAI 兼容角色

```yaml
openai-compatibility:
  - name: "deepseek"
    base-url: "https://api.deepseek.com/v1"
    api-key-entries:
      - api-key: "sk-..."
    models:
      - name: "deepseek-chat"
      - name: "deepseek-reasoner"

auto-router:
  enabled: true
  models:
    - name: "auto"
      fallback:
        provider: "openai-compatible-deepseek"
        model: "deepseek-chat"
      roles:
        - id: "reasoning"
          provider: "openai-compatible-deepseek"
          model: "deepseek-reasoner"
```

## 一句话判断

配置 Auto Router 时，先问两个问题：

1. 这个角色要走哪条 CLIProxyAPI 内部执行链路？答案就是 `provider`。
2. 这条链路下要使用哪个模型或别名？答案就是 `model`。

对于 `name: deepseek` 的 OpenAI 兼容配置，答案通常是：

```yaml
provider: "openai-compatible-deepseek"
model: "deepseek-chat"
```
