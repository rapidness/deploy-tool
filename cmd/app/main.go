// Command app 是服务更新工具的入口。
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"update-app/internal/config"
	"update-app/internal/handler"
	"update-app/internal/middleware"
	"update-app/internal/store"
	"update-app/internal/targets"
)

func main() {
	var (
		configPath = flag.String("config", "config.yaml", "配置文件路径")
		hashPw     = flag.String("hash-password", "", "计算密码哈希并退出")
	)
	flag.Parse()

	if *hashPw != "" {
		fmt.Println(middleware.HashPassword(*hashPw))
		return
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	histPath := filepath.Join(cfg.Log.Path, "history.json")
	st, err := store.New(histPath)
	if err != nil {
		log.Fatalf("初始化历史存储失败: %v", err)
	}

	targetsPath := filepath.Join(cfg.Log.Path, "targets.json")
	ts, err := targets.New(targetsPath)
	if err != nil {
		log.Fatalf("初始化目标路径存储失败: %v", err)
	}

	auth := middleware.NewAuth(cfg.Auth.Username, cfg.Auth.Password, cfg.BasePath)
	h := handler.New(cfg, st, auth, ts)

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	log.Printf("服务更新工具启动，监听 http://%s", addr)
	if err := http.ListenAndServe(addr, h.Routes()); err != nil {
		log.Fatalf("服务启动失败: %v", err)
	}
}

// init 确保日志目录可创建。
func init() {
	_ = os.MkdirAll("logs", 0o755)
}
