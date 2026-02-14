package tools

// RandomString 生成指定长度的确定性字符串（简单实现）
func RandomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[i%len(letters)]
	}
	return string(b)
}

// GenerateMessageID 生成 Anthropic 消息 ID
func GenerateMessageID() string {
	return "msg_" + RandomString(24)
}

// GenerateToolCallID 生成工具调用 ID
func GenerateToolCallID() string {
	return "toolu_" + RandomString(24)
}
