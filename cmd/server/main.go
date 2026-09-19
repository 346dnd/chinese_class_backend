// Command server 启动中华文化教学后端 HTTP 服务。
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"go.uber.org/zap"

	"zhonghuawenhua_backend/internal/api"
	"zhonghuawenhua_backend/internal/config"
	"zhonghuawenhua_backend/internal/handler"
	"zhonghuawenhua_backend/internal/middleware"
	"zhonghuawenhua_backend/internal/service"
	"zhonghuawenhua_backend/pkg/ai"
	"zhonghuawenhua_backend/pkg/db"
	"zhonghuawenhua_backend/pkg/logger"
)

func main() {
	_ = godotenv.Load()

	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "./config.yaml"
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	// 防御性要求：JWT 密钥为空时 HS256 可被任意伪造 token 绕过鉴权，启动即拒绝
	if strings.TrimSpace(cfg.JWT.Secret) == "" {
		log.Fatalf("jwt secret must not be empty")
	}
	if cfg.JWT.ExpireHours <= 0 {
		log.Fatalf("jwt expire_hours must be positive")
	}

	lg, err := logger.New(cfg.Log.Level, cfg.Log.Encoding)
	if err != nil {
		log.Fatalf("init logger: %v", err)
	}
	defer lg.Sync()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool, err := db.NewPostgres(ctx, cfg.Database)
	if err != nil {
		lg.Fatal("init postgres", zap.Error(err))
	}
	defer pool.Close()

	migrationsPath := "./migrations"
	if abs, err := filepath.Abs(migrationsPath); err == nil {
		migrationsPath = abs
	}
	if err := db.RunMigrations(db.DSNForMigrations(cfg.Database), migrationsPath); err != nil {
		lg.Fatal("run migrations", zap.Error(err))
	}

	// 初始化 LLM 客户端（可选，api_key 为空则跳过）
	var llmClient ai.LLMClient
	if cfg.AI.APIKey != "" {
		llmClient, err = ai.NewClient(cfg.AI)
		if err != nil {
			lg.Warn("init ai client failed, ai features disabled", zap.Error(err))
		} else {
			lg.Info("ai client initialized", zap.String("provider", cfg.AI.Provider), zap.String("model", cfg.AI.Model))
		}
	}

	// 从 prompts.dir 目录加载提示词文件（覆盖内联配置）
	cfg.Prompts.LoadPromptsFromDir()

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.AccessLog(lg))

	whitelist := map[string]bool{
		"POST:/api/v1/accounts:login": true,
	}

	// 双认证中间件：JWT + API Key
	authMiddleware := middleware.Auth(middleware.AuthConfig{
		JWTSecret: cfg.JWT.Secret,
		APIKeys:   parseAPIKeys(os.Getenv("API_KEYS")),
		Whitelist: whitelist,
	})

	srv := handler.NewServer(service.NewServices(pool, llmClient, cfg.Prompts, cfg.JWT, cfg.Server.UploadDir, cfg.Server.StorageURL, cfg.OCR, cfg.STT, cfg.TTS))
	api.RegisterHandlersWithOptions(r, srv, api.GinServerOptions{
		Middlewares: []api.MiddlewareFunc{api.MiddlewareFunc(authMiddleware)},
	})

	addr := cfg.Server.Host + ":" + strconv.Itoa(cfg.Server.Port)
	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		lg.Info("server starting", zap.String("addr", addr))
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			lg.Fatal("listen", zap.Error(err))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	lg.Info("shutting down server...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		lg.Error("server shutdown", zap.Error(err))
	}
	lg.Info("server exited")
}

// parseAPIKeys 将逗号分隔的 API_KEY 环境变量解析为 map。
func parseAPIKeys(raw string) map[string]bool {
	m := make(map[string]bool)
	if raw == "" {
		return m
	}
	for _, k := range splitComma(raw) {
		if k = trimSpace(k); k != "" {
			m[k] = true
		}
	}
	return m
}

func splitComma(s string) []string {
	result := make([]string, 0)
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			result = append(result, s[start:i])
			start = i + 1
		}
	}
	result = append(result, s[start:])
	return result
}

func trimSpace(s string) string {
	start := 0
	for start < len(s) && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	end := len(s)
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}
