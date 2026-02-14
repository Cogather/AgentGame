package skill

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/sashabaranov/go-openai"
)

const skillFileName = "SKILL.md"

// InjectAfterSystem 根据配置的目录列表，从每个目录及其子目录读取所有 SKILL.md，
// 将每个文件内容作为一条 user 消息，插入到 system 消息之后；若 skillDirs 为空则不修改。
// 返回新消息切片，原切片未被修改。
func InjectAfterSystem(messages []openai.ChatCompletionMessage, skillDirs []string) ([]openai.ChatCompletionMessage, error) {
	if len(skillDirs) == 0 {
		return messages, nil
	}
	userMsgs, err := loadSkillUserMessages(skillDirs)
	if err != nil {
		return nil, err
	}
	if len(userMsgs) == 0 {
		return messages, nil
	}
	// 找到最后一条 system 消息的下一个位置
	insertAt := 0
	for i := range messages {
		if messages[i].Role == openai.ChatMessageRoleSystem {
			insertAt = i + 1
		}
	}
	// 在 insertAt 处插入所有 skill user 消息
	out := make([]openai.ChatCompletionMessage, 0, len(messages)+len(userMsgs))
	out = append(out, messages[:insertAt]...)
	out = append(out, userMsgs...)
	out = append(out, messages[insertAt:]...)
	return out, nil
}

// loadSkillUserMessages 从目录列表中收集所有 SKILL.md 内容，每个文件一条 user 消息。
func loadSkillUserMessages(skillDirs []string) ([]openai.ChatCompletionMessage, error) {
	var contents []string
	for _, dir := range skillDirs {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		collected, err := collectSkillFilesInDir(dir)
		if err != nil {
			return nil, err
		}
		contents = append(contents, collected...)
	}
	out := make([]openai.ChatCompletionMessage, 0, len(contents))
	for _, c := range contents {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		out = append(out, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleUser,
			Content: c,
		})
	}
	return out, nil
}

// collectSkillFilesInDir 遍历 dir 及其子目录，读取所有名为 SKILL.md 的文件内容。
func collectSkillFilesInDir(dir string) ([]string, error) {
	var contents []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		if d.Name() != skillFileName {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		contents = append(contents, string(data))
		return nil
	})
	return contents, err
}
