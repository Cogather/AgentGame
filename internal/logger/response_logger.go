package logger

import (
	"encoding/json"
	"os"
	"sync"

	"github.com/sashabaranov/go-openai"
)

// ResponseLogger 用于记录模型响应到 response.jsonl
type ResponseLogger struct {
	file     *os.File
	mu       sync.Mutex
	filePath string
}

// NewResponseLogger 创建新的 ResponseLogger
func NewResponseLogger(filePath string) (*ResponseLogger, error) {
	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}

	return &ResponseLogger{
		file:     file,
		filePath: filePath,
	}, nil
}

// Log 记录 ChatCompletionResponse 到文件
func (r *ResponseLogger) Log(resp *openai.ChatCompletionResponse) error {
	if resp == nil {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	jsonData, err := json.Marshal(resp)
	if err != nil {
		return err
	}

	_, err = r.file.WriteString(string(jsonData) + "\n")
	if err != nil {
		return err
	}
	return r.file.Sync()
}

// Close 关闭文件
func (r *ResponseLogger) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.file.Close()
}
