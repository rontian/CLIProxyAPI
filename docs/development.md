# CLIProxyAPI & CPA-Manager macOS 本地开发与验证指南

为了方便您在 macOS 上对本次扩展功能进行本地联调与验证，本文档整理了运行两个项目所需的环境准备、本地启动流程、验证脚本与步骤。

---

## 1. 运行环境准备

在 macOS 上进行开发与测试，您需要准备以下环境：

### 工作区路径约定

本文档假设 `CLIProxyAPI` 与 `CPA-Manager` 两个仓库位于同一个父目录下。不同设备上的物理路径不需要一致，建议在当前终端设置一个工作区变量：

```bash
export CPA_WORKSPACE=/path/to/your/workspace
```

例如目录结构为：

```text
$CPA_WORKSPACE/
  CLIProxyAPI/
  CPA-Manager/
```

如果您已经在某个仓库目录内，也可以直接用相对路径进入另一个仓库，例如从 `CLIProxyAPI` 进入 `../CPA-Manager`。

### Go 运行环境 (后端)
- **版本要求**：Go 1.26+ (项目使用了 Go 1.26 新特性，请确保版本不低于 1.26)。
- **检查命令**：
  ```bash
  go version
  ```
- **安装建议**：若未安装，推荐使用 `Homebrew` 安装：
  ```bash
  brew install go
  ```

### Node.js / Bun 运行环境 (前端)
- **版本要求**：Node.js v18+ 或者是 Bun v1.1+。
- **检查命令**：
  ```bash
  node -v
  npm -v
  # 或者
  bun -v
  ```
- **安装建议**：使用 Homebrew 安装 Node.js：
  ```bash
  brew install node
  ```

---

## 2. CLIProxyAPI 本地启动流程 (后端)

1. **进入目录**：
   ```bash
   cd "$CPA_WORKSPACE/CLIProxyAPI"
   ```
2. **初始化配置**：
   如果目录下没有 `config.yaml`，请先基于模板复制一份：
   ```bash
   cp config.example.yaml config.yaml
   ```
3. **设置管理 Key (Management Key)**：
   打开 `config.yaml` 或新建 `.env` 文件，确保其中包含管理面板认证所需的密钥（例如：`MANAGEMENT_KEY=local_dev_key`），以便前端 CPA-Manager 可以成功调用 API。
4. **编译与运行开发服务器**：
   ```bash
   # 编译运行
   go run ./cmd/server
   ```
   *服务默认会监听在 `8317` 端口。管理 API 基地址为：`http://localhost:8317/v0/management`。*

### 2.1 常用 Makefile 命令

`CLIProxyAPI` 仓库提供了 `Makefile` 作为开发快捷入口：

```bash
make help
make dev
make plugins
make test-auto
make build
make sync-config-dry
make sync-config
```

`make dev` 会先编译维护中的本地插件到 `plugins/<GOOS>/<GOARCH>/`，再启动服务。当前包含 `github-copilot` provider 插件。插件是否加载仍取决于 `config.yaml` 中的 `plugins.enabled` 与 `plugins.configs.<pluginID>.enabled`。

其中 `make sync-config` 只用于本地开发。生产或远程主机不应依赖 `make`、Python 或 Go 环境，应直接执行 `tools/` 下对应平台的预编译二进制。

### 2.2 同步本地配置文件

当 `config.example.yaml` 或 `.env.example` 新增配置项时，不建议直接覆盖已有的 `config.yaml` 或 `.env`。可以使用同步工具只补缺失项，已有配置值不会被改动。

远程 Linux x86_64 主机先预览：

```bash
cd "$CPA_WORKSPACE/CLIProxyAPI"
./tools/sync-config-linux-amd64 --dry-run
```

确认后执行：

```bash
./tools/sync-config-linux-amd64
```

脚本默认同步：

- `config.example.yaml` -> `config.yaml`
- `.env.example` -> `.env`

ARM64 主机使用 `sync-config-linux-arm64`。也可以指定路径：

```bash
./tools/sync-config-linux-amd64 \
  --config /etc/cliproxy/config.yaml \
  --config-example ./config.example.yaml \
  --env /etc/cliproxy/.env \
  --env-example ./.env.example
```

开发机也可以使用 Go 源码或 Python 备用脚本：

```bash
go run ./tools/sync-config --dry-run
./scripts/sync-config.py --dry-run
```

---

## 3. CPA-Manager 本地启动流程 (前端)

1. **进入目录**：
   ```bash
   cd "$CPA_WORKSPACE/CPA-Manager"
   ```
2. **安装依赖**：
   ```bash
   npm install
   # 或者使用 bun:
   bun install
   ```
3. **运行本地开发环境**：
   ```bash
   make dev
   ```
   *该命令会同时启动 Vite 前端和 usage-service。Vite 默认监听 `http://localhost:5173`，usage-service 默认监听 `http://localhost:18317`。*

   如只需要前端页面，不启动 usage-service：
   ```bash
   make dev-web
   ```

   如只需要 usage-service：
   ```bash
   make dev-usage
   ```

4. **网页端登录配置**：
   - 打开浏览器访问 `http://localhost:5173`。
   - 在弹出的配置连接框中输入：
     - **API Base URL**：`http://localhost:8317`
     - **Management Key**：填写您在后端 `config.yaml` 中配置的管理 Key。

### 3.1 常用 Makefile 命令

`CPA-Manager` 仓库同样提供了 `Makefile`：

```bash
make help
make install
make dev
make dev-web
make dev-usage
make type-check
make build
make sync-config-dry
make sync-config
```

`make sync-config` 只用于本地开发，并且只会同步 `.env.example` -> `.env`。生产或远程主机不应依赖 `make`、Python 或 Go 环境，应直接运行预编译二进制：

```bash
cd "$CPA_WORKSPACE/CPA-Manager"
./tools/sync-config-linux-amd64 --skip-yaml --dry-run
./tools/sync-config-linux-amd64 --skip-yaml
```

ARM64 主机使用 `sync-config-linux-arm64`。Python 脚本 `./scripts/sync-config.py` 仅作为开发机备用入口保留。

### 3.5 CPA-Manager 静态文件编译与嵌入部署
如果您希望将编译好的前端代码，直接作为 CLIProxyAPI 自身的控制面板资源部署运行（即通过 `http://localhost:8317/management.html` 访问，不需要在 5173 开启单独的前端服务器）：

1. 在 `CPA-Manager` 目录执行打包命令：
   ```bash
   npm run build
   ```
   *Vite 会将整个 React 应用及样式打包并合并生成一个单文件的 `dist/index.html`。*
2. 创建 CLIProxyAPI 下的静态资源目录并将文件拷贝过去：
   ```bash
   mkdir -p "$CPA_WORKSPACE/CLIProxyAPI/static"
   cp "$CPA_WORKSPACE/CPA-Manager/dist/index.html" "$CPA_WORKSPACE/CLIProxyAPI/static/management.html"
   ```
3. 启动 `CLIProxyAPI` 服务，现在您便可以直接在浏览器访问 `http://localhost:8317/management.html` 来管理代理服务器了。

### 3.6 本地启动额度统计服务 (usage-service)
如果您在本地开发测试中，也希望使用 API 密钥的“别名（Alias）”编辑以及调用数据统计历史图表：

**方法 A：通过 Makefile 启动（推荐，非 Docker 模式）**：
1. **进入 CPA-Manager 目录**：
   ```bash
   cd "$CPA_WORKSPACE/CPA-Manager"
   ```
2. **单独启动 usage-service**：
   ```bash
   make dev-usage
   ```
   *服务启动后会监听在本地的 `18317` 端口。通过 Makefile 启动时，`config.json` 和 SQLite `data/` 目录会统一放在 `CPA-Manager` 仓库根目录。*

**方法 B：直接在 macOS 本地运行（非 Docker 模式）**：
1. **进入 usage-service 目录**：
   ```bash
   cd "$CPA_WORKSPACE/CPA-Manager/usage-service"
   ```
2. **运行 Go 服务**：
   ```bash
   CPA_MANAGER_CONFIG=./config.json go run ./cmd/cpa-manager
   ```
   *服务启动后会监听在本地的 `18317` 端口。该手动方式会在当前 `usage-service` 目录下创建 `config.json` 和 `./data`；如果也想统一到仓库根目录，请使用 `CPA_MANAGER_CONFIG=../config.json go run ./cmd/cpa-manager`。*

**方法 C：使用 Docker 模式启动（标准 Docker Compose 部署）**：
1. **进入 CPA-Manager 目录**：
   ```bash
   cd "$CPA_WORKSPACE/CPA-Manager"
   ```
2. **复制配置并填写环境变量**：
   ```bash
   cp .env.example .env
   # 编辑 .env 文件，填写您的 CPA_UPSTREAM_URL 和 CPA_MANAGEMENT_KEY 等
   ```
3. **使用默认 Docker Compose 启动容器**：
   ```bash
   docker compose up -d --build
   ```
   *CLIProxyAPI 从源码构建 Docker 镜像时会自动编译并打包维护中的本地插件，例如 `github-copilot`。如果直接使用远端预构建镜像，是否包含该插件取决于镜像版本。*

**在 CPA-Manager 界面中配置它**：
1. 登录进入管理端。
2. 点击顶部选项卡中的 **“CPA-Manager 配置”**。
3. 勾选 **“启用数据统计数据库服务 (CPAM)”**。
4. 在服务地址中填入：`http://localhost:18317`，然后点击保存。
5. 此时，API 密钥添加/修改 modal 里的 **“别名”** 输入框便会解锁并恢复为可输入状态。

---

## 4. 功能验证步骤与 Curl 脚本

以下是用来验证本次改造各项新增功能的具体流程：

### 验证 4.1：嵌入模型 (Embedding) 支持测试
通过 curl 提交嵌入向量生成请求。我们应当能看到请求被正常接收并透传到您配置的兼容渠道模型，不再返回 404：

```bash
curl -X POST http://localhost:8317/v1/embeddings \
  -H "Authorization: Bearer <您的用户API-Key>" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "text-embedding-3-small",
    "input": "Hello, world!"
  }'
```

### 验证 4.2：第三方生图模型 (agnes-image-2.1) 免注册识别
通过生图接口提交带有 `image` 关键字的第三方模型。修改后，后端应能基于名字启发式匹配放行，不会提示 `model_not_supported`：

```bash
curl -X POST http://localhost:8317/v1/images/generations \
  -H "Authorization: Bearer <您的用户API-Key>" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "agnes-image-2.1",
    "prompt": "A futuristic city in the style of neon cyberpunk",
    "n": 1,
    "size": "1024x1024"
  }'
```

### 验证 4.3：OAuth 凭证内别名 (Model Aliases) 在管理端编辑保存
1. 打开本地启动的 `CPA-Manager` 页面。
2. 导航至 **“认证凭证 (Auth Files)”** 列表，找到一个 OAuth 类型的凭证文件。
3. 点击 **“支持的模型 (Supported Models)”**。
4. **验证别名编辑功能**：
   - 检查可用模型名旁边是否已经渲染了“输入别名”的文本框。
   - 在 `gemini-3.5-flash-low` 旁输入别名 `gemini-3.5-flash`，并点击下方的 **“保存别名”**。
5. **验证落盘持久化**：
   - 检查对应的 `auths/` 目录下的凭证 `.json` 文件，确认 `"model-aliases"` 字段已被成功 patch 写入：
     ```json
     "model-aliases": [
       { "name": "gemini-3.5-flash-low", "alias": "gemini-3.5-flash" }
     ]
     ```

### 验证 4.4：Auto 模型路由 (Auto Router) 配置与 dry-run

Auto Router 第一版提供一个客户端可见的 `auto` 模型。后端根据配置的角色、主脑模型、匹配关键词和 sticky session 策略，把请求路由到具体 provider/model。详细设计见 `docs/auto-router.md`。

#### 4.4.1 在 CPA-Manager 中配置

1. 启动 `CLIProxyAPI` 与 `CPA-Manager` 后，打开 `http://localhost:5173`。
2. 登录后进入左侧 **“Auto 模型”** 页面。
3. 点击 **“添加 Auto 模型”**，确认或填写以下关键字段：
   - 模型名称：`auto`
   - Fallback Provider / Model：例如 `claude` / `claude-sonnet-4-5`
   - 主脑 Provider / Model：例如 `gemini` / `gemini-2.5-flash`，也可以留空只使用关键词规则
   - 粘性会话：保持启用，TTL 可使用 `30m`
   - 角色：至少配置一个 `coding` 角色，并在匹配关键词中加入 `docker`、`go test`、`stack trace`
4. 点击 **“保存”**。

#### 4.4.2 在 CPA-Manager 中执行 dry-run

dry-run 只预览确定性规则路由，不调用主脑模型，也不会创建新的 sticky session。它用于快速确认某条用户输入会命中哪个角色。

1. 在 **“Auto 模型”** 页面右侧 **“Dry-run 测试”** 区域填写：
   - 请求模型：`auto`
   - Session ID：`preview-session`
   - 用户输入：`new task: debug this docker build failure`
2. 点击 **“运行测试”**。
3. 预期结果应显示命中的角色，例如：
   - `role_id`: `coding`
   - `provider/model`: 配置中 `coding` 角色对应的目标
   - `reason`: 包含 `matched keyword "docker"` 或类似匹配原因

#### 4.4.3 使用 curl 执行 dry-run

```bash
curl -s http://localhost:8317/v0/management/auto-router/dry-run \
  -H "Authorization: Bearer <您的Management Key>" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "auto",
    "source_format": "openai",
    "headers": {
      "X-Session-Id": ["preview-session"]
    },
    "body": {
      "messages": [
        {"role": "user", "content": "new task: debug this docker build failure"}
      ]
    }
  }'
```

典型返回：

```json
{
  "handled": true,
  "decision": {
    "provider": "codex",
    "model": "gpt-5-codex",
    "role_id": "coding",
    "reason": "matched keyword \"docker\"",
    "brain": false,
    "sticky": false
  }
}
```

#### 4.4.4 验证真实 `model=auto` 请求

确认 dry-run 命中预期角色后，可以用真实 OpenAI Chat Completions 请求测试路由执行。此请求会真正调用被路由到的上游模型。

```bash
curl -s http://localhost:8317/v1/chat/completions \
  -H "Authorization: Bearer <您的用户API-Key>" \
  -H "Content-Type: application/json" \
  -H "X-Session-Id: local-auto-test" \
  -d '{
    "model": "auto",
    "messages": [
      {"role": "user", "content": "debug this docker build failure"}
    ]
  }'
```

如需验证 sticky session，可继续使用相同 `X-Session-Id` 发送追问：

```bash
curl -s http://localhost:8317/v1/chat/completions \
  -H "Authorization: Bearer <您的用户API-Key>" \
  -H "Content-Type: application/json" \
  -H "X-Session-Id: local-auto-test" \
  -d '{
    "model": "auto",
    "messages": [
      {"role": "user", "content": "debug this docker build failure"},
      {"role": "assistant", "content": "Please share the Dockerfile and error output."},
      {"role": "user", "content": "那应该怎么修？"}
    ]
  }'
```

如果需要强制重新判定当前 sticky session，可以使用显式切换信号：

```bash
curl -s http://localhost:8317/v1/chat/completions \
  -H "Authorization: Bearer <您的用户API-Key>" \
  -H "Content-Type: application/json" \
  -H "X-Session-Id: local-auto-test" \
  -H "X-Auto-Route-Reset: true" \
  -d '{
    "model": "auto",
    "messages": [
      {"role": "user", "content": "new task: summarize this text"}
    ]
  }'
```

#### 4.4.5 查看和清理 sticky sessions

在 CPA-Manager 的 **“Auto 模型”** 页面右侧 **“运行态会话”** 区域可以查看和清空 sticky session。

也可以使用 curl：

```bash
curl -s http://localhost:8317/v0/management/auto-router/sessions \
  -H "Authorization: Bearer <您的Management Key>"
```

```bash
curl -X DELETE -s http://localhost:8317/v0/management/auto-router/sessions \
  -H "Authorization: Bearer <您的Management Key>"
```

---

## 5. 提交前验证建议

后端改动建议至少执行：

```bash
cd "$CPA_WORKSPACE/CLIProxyAPI"
gofmt -w .
go test ./internal/api/handlers/management ./internal/api ./internal/autorouter ./internal/config ./sdk/api/handlers ./sdk/api/handlers/openai ./sdk/config
go build -buildmode=c-shared -o /tmp/github-copilot-plugin.dylib ./plugins-src/github-copilot/go
go build -o test-output ./cmd/server && rm test-output
```

前端改动建议至少执行：

```bash
cd "$CPA_WORKSPACE/CPA-Manager"
npm run type-check
npm run build
```

当前 `go test ./...` 仍可能在既有无关包失败；如果只验证 Auto Router，本节后端目标测试更有针对性。
