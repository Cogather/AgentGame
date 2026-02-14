# 用户信息管理功能

本文档描述游戏系统中用户信息管理功能的设计与使用方法。

## 功能概述

用户信息管理模块负责维护参与游戏的用户信息，包括用户工号、用户名、队伍名、Agent 访问信息等。每个用户拥有独立的工作空间，用于隔离用户的游戏数据和配置。

## 用户数据模型

### 用户结构

```go
type User struct {
    UserID    string `json:"user_id"`    // 用户工号（唯一标识）
    Username  string `json:"username"`   // 用户名
    TeamName  string `json:"team_name"`  // 队伍名
    AgentIP   string `json:"agent_ip"`   // Agent IP 地址
    AgentPort int    `json:"agent_port"` // Agent 端口号
}
```

### 字段说明

| 字段名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| user_id | string | 是 | 用户工号，作为用户的唯一标识符 |
| username | string | 是 | 用户显示名称 |
| team_name | string | 否 | 所属队伍名称 |
| agent_ip | string | 是 | 用户 Agent 服务的 IP 地址 |
| agent_port | int | 是 | 用户 Agent 服务的端口号 |

## 存储设计

### 工作空间

每个用户拥有独立的工作空间，采用文件系统目录方式进行管理：

- 工作空间根目录：`workspace/`
- 用户工作空间：`workspace/{user_id}/`

用户工作空间命名与用户工号保持一致，确保唯一性。

### 用户清单

维护一个用户工号清单文件 `workspace/users.json`，用于快速加载所有用户信息：

```json
{
  "users": ["user001", "user002", "user003"]
}
```

### 用户数据文件

每个用户的信息存储在独立文件中：`workspace/{user_id}/user.json`

```json
{
  "user_id": "user001",
  "username": "张三",
  "team_name": "红队",
  "agent_ip": "192.168.1.100",
  "agent_port": 8080
}
```

## HTTP API 接口

### 1. 添加用户

**接口地址：** `POST /api/users`

**请求参数：**

```json
{
  "user_id": "user001",
  "username": "张三",
  "team_name": "红队",
  "agent_ip": "192.168.1.100",
  "agent_port": 8080
}
```

**响应示例：**

```json
{
  "code": 0,
  "message": "用户添加成功",
  "data": {
    "user_id": "user001",
    "username": "张三",
    "team_name": "红队",
    "agent_ip": "192.168.1.100",
    "agent_port": 8080
  }
}
```

**错误响应：**

```json
{
  "code": 400,
  "message": "用户工号已存在",
  "data": null
}
```

### 2. 查询用户

**接口地址：** `GET /api/users/{user_id}`

**响应示例（成功）：**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "user_id": "user001",
    "username": "张三",
    "team_name": "红队",
    "agent_ip": "192.168.1.100",
    "agent_port": 8080
  }
}
```

**响应示例（用户不存在）：**

```json
{
  "code": 404,
  "message": "用户不存在",
  "data": null
}
```

### 3. 获取所有用户

**接口地址：** `GET /api/users`

**响应示例：**

```json
{
  "code": 0,
  "message": "success",
  "data": [
    {
      "user_id": "user001",
      "username": "张三",
      "team_name": "红队",
      "agent_ip": "192.168.1.100",
      "agent_port": 8080
    },
    {
      "user_id": "user002",
      "username": "李四",
      "team_name": "蓝队",
      "agent_ip": "192.168.1.101",
      "agent_port": 8080
    }
  ]
}
```

### 4. 更新用户

**接口地址：** `PUT /api/users/{user_id}`

**请求参数：**

```json
{
  "username": "张三（新）",
  "team_name": "蓝队",
  "agent_ip": "192.168.1.200",
  "agent_port": 9090
}
```

**响应示例：**

```json
{
  "code": 0,
  "message": "用户更新成功",
  "data": {
    "user_id": "user001",
    "username": "张三（新）",
    "team_name": "蓝队",
    "agent_ip": "192.168.1.200",
    "agent_port": 9090
  }
}
```

### 5. 删除用户

**接口地址：** `DELETE /api/users/{user_id}`

**响应示例：**

```json
{
  "code": 0,
  "message": "用户删除成功",
  "data": null
}
```

## 使用示例

### 添加用户

```bash
curl -X POST http://localhost:8080/api/users \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "user001",
    "username": "张三",
    "team_name": "红队",
    "agent_ip": "192.168.1.100",
    "agent_port": 8080
  }'
```

### 查询用户

```bash
curl http://localhost:8080/api/users/user001
```

### 获取 Agent 访问地址

通过用户信息可以获取用户的 Agent 访问地址：

```go
user, _ := userManager.GetUser("user001")
agentURL := fmt.Sprintf("http://%s:%d", user.AgentIP, user.AgentPort)
// 结果: http://192.168.1.100:8080
```

## 错误码说明

| 错误码 | 说明 |
|--------|------|
| 0 | 成功 |
| 400 | 请求参数错误 |
| 404 | 用户不存在 |
| 409 | 用户已存在 |
| 500 | 服务器内部错误 |

## 初始化加载

系统启动时会自动从 `workspace/` 目录加载所有用户信息到内存中：

1. 读取 `workspace/users.json` 获取用户工号清单
2. 遍历每个用户工号，加载 `workspace/{user_id}/user.json`
3. 将所有用户信息缓存到内存中，提供快速查询能力

## 数据持久化

- 用户信息实时写入磁盘文件
- 用户清单在添加/删除用户时自动更新
- 工作空间目录在添加用户时自动创建
