package service

import (
	"strings"

	"github.com/sashabaranov/go-openai"
)

// 工具调用标签常量
const ToolCallTag = "<TOOL_CALL_NEEDED>"

// 内容中若包含以下关键字（不区分大小写）也视为需要工具调用
var toolCallKeywords = []string{
	ToolCallTag,
	"function_call",
	"tool_call",
}

// hasToolCallInContent 检查内容是否包含工具调用相关标签或关键字
func hasToolCallInContent(content string) bool {
	if content == "" {
		return false
	}
	lower := strings.ToLower(content)
	for _, kw := range toolCallKeywords {
		if kw == ToolCallTag {
			if strings.Contains(content, kw) {
				return true
			}
		} else if strings.Contains(lower, strings.ToLower(kw)) {
			return true
		}
	}
	return false
}

// HasToolCall 判断响应中是否包含工具调用
// 1. 检查是否有实际的工具调用（tool_calls 字段或 finish_reason）
// 2. 检查内容中是否包含 <TOOL_CALL_NEEDED>、function_call、tool_call 等关键字
func HasToolCall(response *openai.ChatCompletionResponse) bool {
	if len(response.Choices) == 0 {
		return false
	}

	choice := response.Choices[0]

	// 检查实际的工具调用
	if choice.Message.ToolCalls != nil && len(choice.Message.ToolCalls) > 0 {
		return true
	}
	if choice.FinishReason == openai.FinishReasonToolCalls {
		return true
	}

	// 检查内容中的标签或关键字
	return hasToolCallInContent(choice.Message.Content)
}
