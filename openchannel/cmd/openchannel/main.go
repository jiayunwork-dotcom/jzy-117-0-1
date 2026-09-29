// Command openchannel 启动明渠水力核算 HTTP 服务。
//
// 本文件只负责装配与进程生命周期：路由/编解码在 internal/server，
// 断面几何、曼宁迭代、堰流、流态判定、校验分别在各自独立的包中。
// 服务范围仅限明渠均匀流与薄壁矩形堰，不涉及有压管网与流域产汇流。
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"openchannel/internal/server"
)

func main() {
	addr := ":" + envOr("PORT", "8080")

	srv := &http.Server{
		Addr:              addr,
		Handler:           server.New().Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// 监听中断信号，留出 15s 排空在途请求。
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("明渠水力核算服务监听 %s（健康检查 GET /healthz）", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("HTTP 服务异常退出: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("收到关停信号，开始优雅关停")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("优雅关停失败: %v", err)
		_ = srv.Close()
	}
	log.Println("服务已停止")
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
