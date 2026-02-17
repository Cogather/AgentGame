# OpenAI Function / Tool 定义样例（官方 Schema）

以下为 OpenAI API 中 **function calling** 使用的工具定义格式，来源于 [Function calling | OpenAI API](https://platform.openai.com/docs/guides/function-calling)。

**关于「Schema 3.0」**：OpenAI 官方文档未使用「schema 3.0」这一版本号；当前格式为 **type + name + description + parameters（JSON Schema）+ 可选 strict**。若您所指的 schema 3.0 是某份具体规范（如公司内部或行业约定），请提供出处，可据此对齐本仓库的工具定义。

## 单个 function 的字段

| 字段 | 必填 | 说明 |
|------|------|------|
| **type** | 是 | 固定为 `"function"` |
| **name** | 是 | 函数名，如 `get_weather` |
| **description** | 是 | 函数用途说明，供模型决定是否调用 |
| **parameters** | 是 | [JSON Schema](https://json-schema.org/) 描述入参结构 |
| **strict** | 否 | 设为 `true` 时启用严格模式，模型输出必须符合 schema |

## 官方示例：get_weather

```json
{
  "type": "function",
  "name": "get_weather",
  "description": "Retrieves current weather for the given location.",
  "parameters": {
    "type": "object",
    "properties": {
      "location": {
        "type": "string",
        "description": "City and country e.g. Bogotá, Colombia"
      },
      "units": {
        "type": "string",
        "enum": ["celsius", "fahrenheit"],
        "description": "Units the temperature will be returned in."
      }
    },
    "required": ["location", "units"],
    "additionalProperties": false
  },
  "strict": true
}
```

## 传入 API 时的形态：tools 数组

请求时把多个 function 放在 **tools** 数组里，例如：

```json
{
  "model": "gpt-4o",
  "messages": [...],
  "tools": [
    {
      "type": "function",
      "name": "get_weather",
      "description": "Retrieves current weather for the given location.",
      "parameters": {
        "type": "object",
        "properties": {
          "location": { "type": "string", "description": "City and country e.g. Bogotá, Colombia" },
          "units": { "type": "string", "enum": ["celsius", "fahrenheit"], "description": "Units the temperature will be returned in." }
        },
        "required": ["location", "units"],
        "additionalProperties": false
      },
      "strict": true
    }
  ]
}
```

## parameters 约定（JSON Schema）

- **type**：一般为 `"object"`
- **properties**：键为参数名，值为 `{ "type": "string"|"integer"|"number"|"boolean", "description": "...", "enum": [...] }` 等
- **required**：必填参数名数组
- **additionalProperties**：严格模式下建议为 `false`

## Strict 模式说明

- 开启 `"strict": true` 时，模型生成的工具参数会严格符合上述 schema。
- 要求：`parameters` 中每个 object 都要设 `additionalProperties: false`；可选字段可用 `"type": ["string", "null"]` 等方式表示。

## 与本仓库工具列表的对应关系

`fake_app_agent_tools.json` 中每一项是 **仅含 name、description、parameters（及可选 api）** 的简化结构。  
若要按 OpenAI 规范直接传入 API，需要先包成上述格式，例如：

```javascript
// 将本地工具项转为 OpenAI tools 数组
const tools = fakeAppTools.map(t => ({
  type: "function",
  name: t.name,
  description: t.description,
  parameters: t.parameters,
  ...(t.strict !== false && { strict: true })
}));
```

其中 **parameters** 可直接使用本仓库中的 `parameters` 对象；**query/body 区分**由本仓库的 **api.path_params / api.body_params** 在执行器侧使用，不写入 OpenAI 的 schema。
