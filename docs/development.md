# CLIProxyAPI & CPA-Manager macOS 本地开发与验证指南

为了方便您在 macOS 上对本次扩展功能进行本地联调与验证，本文档整理了运行两个项目所需的环境准备、本地启动流程、验证脚本与步骤。

---

## 1. 运行环境准备

在 macOS 上进行开发与测试，您需要准备以下环境：

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
   cd /Volumes/MacintoshWD/rontian/CLIProxyAPI
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

---

## 3. CPA-Manager 本地启动流程 (前端)

1. **进入目录**：
   ```bash
   cd /Volumes/MacintoshWD/rontian/CPA-Manager
   ```
2. **安装依赖**：
   ```bash
   npm install
   # 或者使用 bun:
   bun install
   ```
3. **运行开发服务器**：
   ```bash
   npm run dev
   # 或者使用 bun:
   bun run dev
   ```
   *Vite 开发服务器默认启动在 `http://localhost:5173`。*

4. **网页端登录配置**：
   - 打开浏览器访问 `http://localhost:5173`。
   - 在弹出的配置连接框中输入：
     - **API Base URL**：`http://localhost:8317`
     - **Management Key**：填写您在后端 `config.yaml` 中配置的管理 Key。

### 3.5 CPA-Manager 静态文件编译与嵌入部署
如果您希望将编译好的前端代码，直接作为 CLIProxyAPI 自身的控制面板资源部署运行（即通过 `http://localhost:8317/management.html` 访问，不需要在 5173 开启单独的前端服务器）：

1. 在 `CPA-Manager` 目录执行打包命令：
   ```bash
   npm run build
   ```
   *Vite 会将整个 React 应用及样式打包并合并生成一个单文件的 `dist/index.html`。*
2. 创建 CLIProxyAPI 下的静态资源目录并将文件拷贝过去：
   ```bash
   mkdir -p /Volumes/MacintoshWD/rontian/CLIProxyAPI/static
   cp /Volumes/MacintoshWD/rontian/CPA-Manager/dist/index.html /Volumes/MacintoshWD/rontian/CLIProxyAPI/static/management.html
   ```
3. 启动 `CLIProxyAPI` 服务，现在您便可以直接在浏览器访问 `http://localhost:8317/management.html` 来管理代理服务器了。

### 3.6 本地启动额度统计服务 (usage-service)
如果您在本地开发测试中，也希望使用 API 密钥的“别名（Alias）”编辑以及调用数据统计历史图表：

**方法 A：直接在 macOS 本地运行（推荐，非 Docker 模式）**：
1. **进入 usage-service 目录**：
   ```bash
   cd /Volumes/MacintoshWD/rontian/CPA-Manager/usage-service
   ```
2. **运行 Go 服务**：
   ```bash
   CPA_MANAGER_CONFIG=./config.json go run ./cmd/cpa-manager
   ```
   *服务启动后会监听在本地的 `18317` 端口。同时会在当前目录下创建 `config.json` 配置文件及存放 SQLite 数据库的 `./data` 目录。*

**方法 B：使用 Docker 模式启动（标准 Docker Compose 部署）**：
1. **进入 CPA-Manager 目录**：
   ```bash
   cd /Volumes/MacintoshWD/rontian/CPA-Manager
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
