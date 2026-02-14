package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

func main() {
	baseURL := "http://localhost:8080"
	
	fmt.Println("=" + strings.Repeat("=", 60))
	fmt.Println("代理服务测试客户端")
	fmt.Println("=" + strings.Repeat("=", 60))
	fmt.Println()

	// 测试1: 非流式聊天模型
	fmt.Println("测试1: 非流式聊天模型请求")
	testNonStreamChat(baseURL)
	fmt.Println()

	// 测试2: 流式聊天模型
	fmt.Println("测试2: 流式聊天模型请求")
	testStreamChat(baseURL)
	fmt.Println()

	// 测试3: 非流式工作模型
	fmt.Println("测试3: 非流式工作模型请求")
	testNonStreamWork(baseURL)
	fmt.Println()

	// 测试4: 流式工作模型
	fmt.Println("测试4: 流式工作模型请求")
	testStreamWork(baseURL)
	fmt.Println()

	fmt.Println("=" + strings.Repeat("=", 60))
	fmt.Println("✅ 所有测试完成！")
	fmt.Println("=" + strings.Repeat("=", 60))
}

func testNonStreamChat(baseURL string) {
	reqBody := map[string]interface{}{
		"model": "caht",
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": "你好，请简单回复一下。",
			},
		},
		"stream": false,
		"max_tokens": 50,
	}

	resp, err := sendRequest(baseURL+"/v1/chat/completions", reqBody)
	if err != nil {
		fmt.Printf("❌ 请求失败: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("❌ 响应错误: %d, %s\n", resp.StatusCode, string(body))
		return
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		fmt.Printf("❌ 解析响应失败: %v\n", err)
		return
	}

	fmt.Printf("✅ 请求成功\n")
	if choices, ok := result["choices"].([]interface{}); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]interface{}); ok {
			if message, ok := choice["message"].(map[string]interface{}); ok {
				if content, ok := message["content"].(string); ok {
					fmt.Printf("   响应内容: %s\n", content)
				}
			}
		}
	}
}

func testStreamChat(baseURL string) {
	reqBody := map[string]interface{}{
		"model": "chat",
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": "你好，请简单回复一下。",
			},
		},
		"stream": true,
		"max_tokens": 50,
	}

	resp, err := sendRequest(baseURL+"/v1/chat/completions", reqBody)
	if err != nil {
		fmt.Printf("❌ 请求失败: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("❌ 响应错误: %d, %s\n", resp.StatusCode, string(body))
		return
	}

	fmt.Printf("✅ 流式响应开始:\n")
	buffer := make([]byte, 1024)
	for {
		n, err := resp.Body.Read(buffer)
		if n > 0 {
			fmt.Print(string(buffer[:n]))
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Printf("\n❌ 读取流失败: %v\n", err)
			break
		}
	}
	fmt.Println()
}

func testNonStreamWork(baseURL string) {
	reqBody := map[string]interface{}{
		"model": "work",
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": "你好，请简单回复一下。",
			},
		},
		"stream": false,
		"max_tokens": 50,
	}

	resp, err := sendRequest(baseURL+"/v1/chat/completions", reqBody)
	if err != nil {
		fmt.Printf("❌ 请求失败: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("❌ 响应错误: %d, %s\n", resp.StatusCode, string(body))
		return
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		fmt.Printf("❌ 解析响应失败: %v\n", err)
		return
	}

	fmt.Printf("✅ 请求成功\n")
	if choices, ok := result["choices"].([]interface{}); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]interface{}); ok {
			if message, ok := choice["message"].(map[string]interface{}); ok {
				if content, ok := message["content"].(string); ok {
					fmt.Printf("   响应内容: %s\n", content)
				}
			}
		}
	}
}

func testStreamWork(baseURL string) {
	reqBody := map[string]interface{}{
		"model": "work",
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": "你好，请简单回复一下。",
			},
		},
		"stream": true,
		"max_tokens": 50,
	}

	resp, err := sendRequest(baseURL+"/v1/chat/completions", reqBody)
	if err != nil {
		fmt.Printf("❌ 请求失败: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("❌ 响应错误: %d, %s\n", resp.StatusCode, string(body))
		return
	}

	fmt.Printf("✅ 流式响应开始:\n")
	buffer := make([]byte, 1024)
	for {
		n, err := resp.Body.Read(buffer)
		if n > 0 {
			fmt.Print(string(buffer[:n]))
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Printf("\n❌ 读取流失败: %v\n", err)
			break
		}
	}
	fmt.Println()
}

func sendRequest(url string, body interface{}) (*http.Response, error) {
	jsonData, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	return client.Do(req)
}
