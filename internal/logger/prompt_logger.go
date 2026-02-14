package logger

import (
	"encoding/json"
	"os"
	"sync"

	"github.com/sashabaranov/go-openai"
)

// PromptLogger 用于记录用户请求的 messages（仅最后一条）
type PromptLogger struct {
	file     *os.File
	mu       sync.Mutex
	filePath string
}

// NewPromptLogger 创建新的 PromptLogger
func NewPromptLogger(filePath string) (*PromptLogger, error) {
	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}

	return &PromptLogger{
		file:     file,
		filePath: filePath,
	}, nil
}

// Log 记录 messages 到文件（只保存最后一条消息，不保存 tools）
func (p *PromptLogger) Log(messages []openai.ChatCompletionMessage) error {
	if len(messages) == 0 {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	// lastOnly := messages[len(messages)-1:]
	data := map[string]interface{}{
		"messages": messages,
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	_, err = p.file.WriteString(string(jsonData) + "\n")
	if err != nil {
		return err
	}
	return p.file.Sync()
}

// Close 关闭文件
func (p *PromptLogger) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.file.Close()
}
