// Package middleware 提供 HTTP 中间件。
package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"

	"zhonghuawenhua_backend/pkg/httpresp"
	"zhonghuawenhua_backend/pkg/jwt"
)

// AuthConfig 认证配置。
type AuthConfig struct {
	JWTSecret  string
	APIKeys    map[string]bool // 合法的 API Key 集合
	Whitelist  map[string]bool // 免鉴权路径 METHOD:PATH
}

// Auth 返回支持双认证的中间件：JWT（Bearer Token）或 API Key（X-API-Key）。
// 优先级：JWT > API Key。whitelist 路径直接放行。
func Auth(cfg AuthConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.Request.Method + ":" + c.Request.URL.Path
		if cfg.Whitelist[key] {
			c.Next()
			return
		}

		// 方式一：JWT Bearer Token
		authHeader := c.GetHeader("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			tokenString := strings.TrimPrefix(authHeader, "Bearer ")
			if claims, err := jwt.Parse(cfg.JWTSecret, tokenString); err == nil {
				c.Set("user_id", claims.UserID)
				c.Set("auth_type", "jwt")
				c.Next()
				return
			}
		}

		// 方式二：API Key（用于服务间调用或外部集成）
		if apiKey := c.GetHeader("X-API-Key"); apiKey != "" && cfg.APIKeys[apiKey] {
			c.Set("auth_type", "api_key")
			c.Next()
			return
		}

		httpresp.Unauthorized(c, "missing or invalid authorization")
		c.Abort()
	}
}

// JWTAuth 纯 JWT 中间件（保留兼容）。
func JWTAuth(secret string, whitelist map[string]bool) gin.HandlerFunc {
	return Auth(AuthConfig{
		JWTSecret: secret,
		Whitelist: whitelist,
	})
}
