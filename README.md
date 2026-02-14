# OpenClawProxy - AI 代理服务

基于 Go 实现的 OpenAI 协议代理，支持**双模型智能路由**与**工具调用前处理**：请求工作模型时，可先由聊天模型判断是否需要工具，再决定走工作模型或直接返回聊天结果，从而节省工作模型调用成本。

## 功能特性

- **OpenAI 协议兼容**：使用 `github.com/sashabaranov/go-openai`，支持标准 `/v1/chat/completions` 请求（messages、tools、tool_choice、temperature、max_tokens、stream 等）。
- **双模型配置**：可配置**聊天模型**（轻量、低成本）与**工作模型**（支持工具/思考等），通过请求中的 `model` 与配置中的 `model_name` 匹配决定路由。
- **前处理（可选）**：当请求的是工作模型、且最后一条消息为 user、且开启 `preprocess_enabled` 时，先调用聊天模型（不传 tools）判断是否需要工具；若判断需要则再调用工作模型，否则直接返回聊天模型结果（流式请求会包装成 SSE 流）。
- **流式 / 非流式**：均支持；流式时工作模型直接转发上游 SSE，非流式转流式通过内部包装器输出。
- **Moonshot 兼容**：当工作模型 `base_url` 包含 `moonshot` 时，客户端会自动为 assistant 消息补全 `reasoning_content` 并设置 `reasoning: false`，便于对接 Moonshot thinking 模型。
- **可选日志**：可配置将请求 messages（仅最后一条）写入 `prompt_log_file`、将完整响应写入 `response_log_file`（JSONL），不配置或文件名为空则不落盘。

## 项目结构

```
├── cmd/                      # 命令行与测试
│   ├── test_client/         # 客户端测试
│   ├── test_dedup/           # 去重相关测试
│   ├── test_kimi/            # Kimi API 测试
│   ├── test_porxy/           # 代理测试
│   └── test_stream_sync/     # 流式同步测试
├── client/
│   └── openai_client.go      # OpenAI 协议客户端（含 Moonshot reasoning_content 扩展）
├── config/
│   └── config.go             # 配置结构定义与加载
├── handler/
│   └── handler.go            # HTTP 路由与 /v1/chat/completions、/health 处理
├── internal/
│   ├── logger/               # 请求/响应日志
│   │   ├── prompt_logger.go  # 记录 messages（最后一条）
│   │   └── response_logger.go # 记录 ChatCompletionResponse
│   └── stream/
│       └── wrappers.go       # 非流式转流式包装
├── service/
│   ├── proxy_service.go      # 代理与路由、前处理逻辑
│   └── tool_detector.go      # 工具调用检测（标签与关键字）
├── config.yaml
├── go.mod
└── main.go
```

## 配置说明

编辑 `config.yaml`：

| 配置项 | 说明 |
|--------|------|
| `preprocess_enabled` | 是否启用前处理。`true`：请求工作模型且最后一条为 user 时先走聊天模型判断；`false`：直接使用工作模型。 |
| `chat_model` | 聊天模型（用于前处理判断或直接聊天请求）。 |
| `work_model` | 工作模型（支持工具/思考，可选前处理）。 |
| `server` | 服务监听地址。 |
| `logging` | 可选；不配置或文件名为空则不写日志。 |

每个模型块：

| 字段 | 说明 |
|------|------|
| `base_url` | 上游 API 地址（如 `https://api.openai.com/v1`、`https://api.moonshot.cn/v1`）。 |
| `api_key` | 上游 API Key。 |
| `model_name` | **用户请求时使用的模型名**，用于判断走 chat 还是 work（与请求体中的 `model` 匹配）。 |
| `model_id` | **实际请求上游时使用的模型 ID**。 |

示例（请替换为真实 base_url / api_key / model_id）：

```yaml
preprocess_enabled: true

chat_model:
  base_url: "https://api.openai.com/v1"
  api_key: "your-chat-api-key"
  model_name: "chat"        # 客户端请求 model=chat 时走聊天模型
  model_id: "gpt-4"         # 实际请求上游的模型

work_model:
  base_url: "https://api.moonshot.cn/v1"
  api_key: "your-work-api-key"
  model_name: "work"        # 客户端请求 model=work 时走工作模型（会触发前处理逻辑）
  model_id: "kimi-k2.5"

server:
  port: 8080
  host: "0.0.0.0"

# 可选：不配置或文件名为空则不保存
# logging:
#   prompt_log_file: "prompt.jsonl"
#   response_log_file: "response.jsonl"
```

## 安装与运行

1. 环境：Go 1.21+（参考 `go.mod`）。
2. 安装依赖：
   ```bash
   go mod download
   ```
3. 修改 `config.yaml`，填入各模型的 `base_url`、`api_key`、`model_name`、`model_id`。
4. 启动：
   ```bash
   go run main.go
   ```
   或指定配置文件：
   ```bash
   go run main.go /path/to/config.yaml
   ```

## API 端点

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/health` | 健康检查，返回 `{"status":"ok"}`。 |
| POST | `/v1/chat/completions` | 与 OpenAI 一致的聊天完成接口。 |

**路由规则**：

- 请求体中的 `model` 与 `work_model.model_name` 一致 → 视为工作模型请求（若开启前处理且最后一条为 user，会先走聊天模型判断）。
- 否则 → 使用聊天模型。

请求体格式与 OpenAI 一致，例如：

```json
{
  "model": "work",
  "messages": [{"role": "user", "content": "北京今天天气怎么样？"}],
  "tools": [...],
  "stream": true,
  "temperature": 0.7
}
```

## 工作流程（前处理开启时）

当请求为工作模型且最后一条消息为 user 时：

1. 复制请求，去掉 `tools` / `tool_choice`，改为使用聊天模型、非流式调用。
2. 在系统消息中追加一段「工具调用判断规则」说明，要求模型：仅需对话则正常回复且不输出 `<TOOL_CALL_NEEDED>`；需要工具时在回复中包含一次 `<TOOL_CALL_NEEDED>`。
3. 根据聊天模型响应做**工具调用检测**（见下）。
4. 若检测到需要工具 → 用原始请求（含 tools）调用工作模型，流式则直接转发 SSE。
5. 若未检测到 → 直接使用聊天模型结果；若用户请求为流式，则通过包装器将非流式结果转成 SSE 流返回。

## 工具调用检测

`HasToolCall` 在以下任一成立时视为「需要工具」：

- 响应中 `choices[0].message.tool_calls` 非空；
- `choices[0].finish_reason` 为 `tool_calls`；
- 响应内容（`content`）包含：
  - 标签：`<TOOL_CALL_NEEDED>`
  - 或关键字（不区分大小写）：`function_call`、`tool_call`

前处理阶段依赖聊天模型在「需要工具」时输出 `<TOOL_CALL_NEEDED>` 等上述之一，以触发工作模型调用。

## 配置 OpenClaw 使用本代理

[OpenClaw](https://github.com/openclaw/openclaw) 通过 OpenAI 兼容接口调用模型。将本代理的地址配置为 OpenClaw 的模型提供商即可。

**配置文件路径**：

- Windows: `C:\Users\<用户名>\.openclaw\openclaw.json`
- macOS / Linux: `~/.openclaw/openclaw.json`

**配置要点**：

1. 在 `models.providers` 下新增一个提供商（如 `local-models`）：
   - `baseUrl`：本代理的 base URL，需带 `/v1`，例如本机 8080 端口为 `http://127.0.0.1:8080/v1`
   - `apiKey`：任意字符串即可（本代理不校验该 key，上游请求使用 `config.yaml` 中的 api_key）
   - `api`：固定为 `"openai-completions"`
   - `models`：模型列表，其中 `id` / `name` 需与代理里 `config.yaml` 的 `model_name` 一致（如 `work` 或 `chat`）

2. 在 `agents.defaults.model.primary` 中指定默认模型，格式为 `"<提供商名>/<模型 id>"`，例如 `"local-models/work"`。

**示例**（仅保留与代理相关的部分，其余键保持你原有配置即可）：

```json
{
  "models": {
    "mode": "merge",
    "providers": {
      "local-models": {
        "baseUrl": "http://127.0.0.1:8080/v1",
        "apiKey": "local-key",
        "api": "openai-completions",
        "models": [
          {
            "id": "work",
            "name": "work",
            "reasoning": false,
            "input": ["text"],
            "contextWindow": 128000,
            "maxTokens": 8192
          }
        ]
      }
    }
  },
  "agents": {
    "defaults": {
      "model": { "primary": "local-models/work" }
    }
  }
}
```

确保本代理已启动（如 `go run main.go` 且 `config.yaml` 中 `server.port` 为 8080），且 `config.yaml` 里 `work_model.model_name` 为 `work`，这样 OpenClaw 请求 `model=work` 时会走本代理并触发前处理逻辑。若需同时暴露聊天模型，在 `models` 中再增加一项 `id`/`name` 为 `chat` 的模型，并在需要时将 `primary` 改为 `"local-models/chat"`。

## 依赖

- `github.com/sashabaranov/go-openai` - OpenAI Go SDK
- `github.com/gorilla/mux` - HTTP 路由
- `gopkg.in/yaml.v3` - YAML 解析
