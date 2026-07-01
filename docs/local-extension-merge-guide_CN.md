# CLIProxyAPI 本地扩展合并指南

本文用于在 `main` 同步官方仓库后，将官方变更合并到本地扩展分支 `main_ai`。目标是让本地扩展保持可维护，而不是在冲突时逐行猜测。

## 分支模型

- `main`：只跟随官方仓库，不直接承载本地扩展。
- `main_ai`：本地扩展分支，包括 embedding、图片/视频生成、OAuth 模型别名、Auto Router 等能力。
- 同步节奏：先更新 `main`，再把 `main` merge 到 `main_ai`。

推荐流程：

```bash
git checkout main
git pull upstream main

git checkout main_ai
git merge main
```

如果官方远端不是 `upstream`，替换为实际 remote 名称。

## 本地扩展总览

| 扩展                      | 主要配置/API                                                                                                                         | 重点文件                                                                                                                             | 合并关注点                                                                                         |
| ------------------------- | ------------------------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------ | -------------------------------------------------------------------------------------------------- |
| Embeddings 端点           | `/v1/embeddings`                                                                                                                     | `internal/api`、`internal/runtime/executor`、相关 translator                                                                         | 官方若新增 embeddings 路由或模型能力，优先保留官方通用逻辑，再接回本地兼容行为                     |
| 图片生成                  | `/v1/images/*`、`disable-image-generation`、`custom-image-model-keywords`、`gpt-image-2-base-model`                                  | `internal/api/server.go`、`internal/runtime/executor`、`internal/translator/*`、`config.example.yaml`                                | 注意模型识别、禁用策略、Responses/Chat 图片输出转换是否被官方重写                                  |
| 视频生成                  | `/v1/videos/*`、`/openai/v1/videos/*`、`video-result-auth-cache-ttl`、`custom-video-model-keywords`                                  | `internal/api/server.go`、video handlers、runtime executor、日志路径识别                                                             | 官方若新增视频 API，确认 native/OpenAI 兼容路径是否仍区分正确                                      |
| OAuth 模型别名            | `oauth-model-alias`、auth JSON `model-aliases`                                                                                       | `internal/config`、auth file handling、model listing/routing                                                                         | 合并时确认全局别名、每认证文件别名、`force-mapping`、`fork` 语义没有被覆盖                         |
| OpenAI 兼容提供商模型别名 | provider `models[].alias`                                                                                                            | `internal/config`、provider selection、model registry/listing                                                                        | 一个 provider 可暴露多个别名；避免官方模型列表重构后丢失 alias 映射                                |
| Auto Router               | `auto-router.models`、`auto-router.role-presets`、`auto-router.models[].policy`、`roles[].candidates`、`/v0/management/auto-router*` | `internal/autorouter`、`internal/config/auto_router.go`、`internal/api/handlers/management/auto_router.go`、`internal/api/server.go` | 官方若也实现自动路由，需要产品级合并判断，不要简单选 ours/theirs；候选池与策略字段必须保持向后兼容 |
| GitHub Copilot Provider 插件 | `plugins.configs.github-copilot`、provider key `github-copilot`、auth JSON `type=github-copilot`                                  | `plugins-src/github-copilot/go`、`docs/github-copilot-provider-plugin_CN.md`、`config.example.yaml`                                  | 保持为插件 provider，不把 Copilot 内部协议下沉到核心路由或 translator；协议变化优先更新插件        |

## 冲突处理顺序

1. 先解决构建级冲突：Go import、类型、路由注册、配置结构。
2. 再解决行为级冲突：模型选择、别名映射、图片/视频端点、Auto Router 路由时机。
3. 再解决配置示例和文档冲突：`config.example.yaml`、`docs/*`。
4. 最后跑验证，确认功能仍能闭环。

## 常见高风险文件

- `internal/api/server.go`
- `internal/config/config.go`
- `internal/config/sdk_config.go`
- `internal/config/auto_router.go`
- `internal/api/handlers/management/auto_router.go`
- `internal/runtime/executor/*`
- `internal/translator/*`
- `config.example.yaml`
- `docs/development.md`
- `docs/auto-router.md`
- `docs/provider-model-targets_CN.md`

## 合并判断规则

- 官方新增通用能力时，优先保留官方实现，把本地扩展改成兼容层。
- 本地配置字段已经被用户部署使用时，不要直接重命名或删除；需要保留向后兼容。
- 如果官方实现和本地扩展功能重叠，先列出差异，再决定迁移、兼容或保留本地实现。
- 不要用整文件 `ours` 或 `theirs` 处理配置、路由、translator、executor 冲突。
- 对 `internal/translator/` 的冲突，必须结合上游协议变化和本地功能一起处理，避免只让测试通过但破坏格式转换。

## 最小验证清单

每次合并官方变更到 `main_ai` 后至少运行：

```bash
gofmt -w .
go test ./internal/config ./internal/api/handlers/management
go build -o test-output ./cmd/server && rm test-output
```

如果本次冲突涉及图片、视频、OAuth 别名、translator 或 executor，还应增加对应包测试，例如：

```bash
go test ./internal/api ./internal/runtime/executor ./internal/translator/...
```

如果本次冲突涉及 GitHub Copilot 插件，还应至少验证插件可编译：

```bash
go build -buildmode=c-shared -o /tmp/github-copilot-plugin.dylib ./plugins-src/github-copilot/go
```

Linux 环境可把输出后缀替换为 `.so`。

`go test ./...` 仍应作为最终目标；如果存在已知非本次引入的失败，需要在合并记录中说明。

## 任务和提交规则

- 开始合并或功能任务前，先更新 `docs/tasks.md`，新增计划项并标注状态。
- 过程中随着完成度更新任务状态，不把 `tasks.md` 留成过期状态。
- 提交信息使用带类型的中文说明，例如：
  - `feat(auto-router): 支持自定义角色预设`
  - `fix(video): 修复 OpenAI 视频结果绑定`
  - `docs(merge): 新增本地扩展合并指南`
  - `chore(config): 同步示例配置`

## 后续维护

新增本地扩展时，需要同步更新本文：

- 扩展名称和用途
- 配置/API
- 主要文件
- 官方合并时的风险点
- 必跑验证命令
