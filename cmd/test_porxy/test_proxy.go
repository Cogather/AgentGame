package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

const (
	WORK_MODEL_BASE_URL = "https://api.deepseek.com"
	WORK_MODEL_API_KEY  = "sk-a6b323ff062d46ee97361e4f4dd6983c"
	WORK_MODEL_ID       = "deepseek-chat"
)

func main() {
	http.HandleFunc("/v1/chat/completions", chatCompletions)
	http.HandleFunc("/health", healthCheck)

	log.Println("=" + strings.Repeat("=", 49))
	log.Println("OpenAI 代理服务启动")
	log.Printf("工作模型 Base URL: %s", WORK_MODEL_BASE_URL)
	log.Printf("工作模型 ID: %s", WORK_MODEL_ID)
	log.Println("=" + strings.Repeat("=", 49))
	log.Println("\n注意：请确保已修改 WORK_MODEL_API_KEY 为实际的 API Key")
	log.Println("=" + strings.Repeat("=", 49))

	log.Println("服务启动在 :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal(err)
	}
}

func healthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func chatCompletions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 读取请求体
	var reqData map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&reqData); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// 修改模型 ID
	reqData["model"] = WORK_MODEL_ID

	// 检查是否是流式请求
	isStream, _ := reqData["stream"].(bool)

	// 准备请求体
	reqBody, err := json.Marshal(reqData)
	if err != nil {
		http.Error(w, "Failed to marshal request", http.StatusInternalServerError)
		return
	}

	// 创建上游请求
	url := WORK_MODEL_BASE_URL + "/chat/completions"
	httpReq, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		http.Error(w, "Failed to create request", http.StatusInternalServerError)
		return
	}

	// 设置请求头
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+WORK_MODEL_API_KEY)
	if isStream {
		httpReq.Header.Set("Accept", "text/event-stream")
	}

	// 发送请求
	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to send request: %v", err), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	// 检查响应状态
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		w.WriteHeader(resp.StatusCode)
		w.Write(body)
		return
	}

	if isStream {
		// 流式响应
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
		
		// 立即发送响应头
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming not supported", http.StatusInternalServerError)
			return
		}
		flusher.Flush()

		// 直接转发原始数据
		chunkCount := 0
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Bytes()
			if len(line) > 0 {
				chunkCount++
				// 转发原始行（包括 data: 前缀）
				w.Write(line)
				w.Write([]byte("\n"))
				flusher.Flush()

				// 每 10 个 chunk 打印一次日志
				if chunkCount%10 == 0 {
					log.Printf("[流式响应] 已转发 %d 个 chunk", chunkCount)
				}
			} else {
				// 空行也需要转发（SSE 格式要求）
				w.Write([]byte("\n"))
				flusher.Flush()
			}
		}

		if err := scanner.Err(); err != nil && err != io.EOF {
			log.Printf("[错误] 读取流式响应失败: %v", err)
		}

		log.Printf("[流式响应] 完成，共转发 %d 个 chunk", chunkCount)
	} else {
		// 非流式响应
		w.Header().Set("Content-Type", "application/json")
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			http.Error(w, "Failed to read response", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(resp.StatusCode)
		w.Write(body)
	}
}
