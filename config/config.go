package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config 应用配置
type Config struct {
	ChatModel ModelConfig `yaml:"chat_model"`
	WorkModel ModelConfig `yaml:"work_model"`
	Server    ServerConfig `yaml:"server"`
	// PreprocessEnabled 是否启用前处理：工作模型请求时先用聊天模型判断是否需要工具调用
	PreprocessEnabled bool          `yaml:"preprocess_enabled"`
	Logging           LoggingConfig `yaml:"logging"`
	// SkillDirs 技能目录列表，从每个目录读取所有 SKILL.md，每条文件内容作为一条 user 消息插入 system 之后；为空或未配置则不注入
	SkillDirs []string `yaml:"skill_dirs"`
}

// LoggingConfig 日志文件配置，文件名为空则不保存对应日志
type LoggingConfig struct {
	PromptLogFile   string `yaml:"prompt_log_file"`   // prompt 日志文件名，空则不保存
	ResponseLogFile string `yaml:"response_log_file"` // response 日志文件名，空则不保存
}

// ModelConfig 模型配置
type ModelConfig struct {
	BaseURL   string `yaml:"base_url"`
	APIKey    string `yaml:"api_key"`
	ModelName string `yaml:"model_name"` // 用户请求使用的模型名，用于判断 chat/work 路由
	ModelID   string `yaml:"model_id"`   // 实际请求远端 API 的模型 ID
	APIFormat string `yaml:"api_format"` // API 格式: openai 或 anthropic，默认 openai
}

// ServerConfig 服务器配置
type ServerConfig struct {
	Port string `yaml:"port"`
	Host string `yaml:"host"`
}

// LoadConfig 从文件加载配置
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	return &config, nil
}
