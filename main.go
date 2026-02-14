package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	gatewayconfig "ocProxy/gateway/config"
	"ocProxy/gateway/handler"
	gatewayservice "ocProxy/gateway/service"

	"github.com/gorilla/mux"
)

func main() {
	// 加载配置
	cfgPath := "config.yaml"
	if len(os.Args) > 1 {
		cfgPath = os.Args[1]
	}

	cfg, err := gatewayconfig.LoadConfig(cfgPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// 初始化 Gateway 模块（模型访问）
	proxyService := gatewayservice.NewProxyService(cfg)

	// 创建处理器
	h, err := handler.NewHandler(proxyService, cfg)
	if err != nil {
		log.Fatalf("创建处理器失败: %v", err)
	}
	defer h.Close()

	// 设置路由
	r := mux.NewRouter()
	h.SetupRoutes(r)

	// 创建 HTTP 服务器
	addr := fmt.Sprintf("%s:%s", cfg.Server.Host, cfg.Server.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0, // 流式(SSE)响应需要长时间写入，不能设置超时
		IdleTimeout:  120 * time.Second, // 空闲连接超时时间
	}

	// 启动服务器
	go func() {
		log.Printf("服务器启动在 %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("服务器启动失败: %v", err)
		}
	}()

	// 优雅关闭
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("正在关闭服务器...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("服务器关闭失败: %v", err)
	}

	log.Println("服务器已关闭")
}
