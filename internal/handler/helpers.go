package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"zhonghuawenhua_backend/internal/service"
	"zhonghuawenhua_backend/pkg/httpresp"
)

// currentUserID 从 gin.Context 提取 JWT 中间件注入的 user_id。
func currentUserID(c *gin.Context) (int64, bool) {
	v, exists := c.Get("user_id")
	if !exists {
		return 0, false
	}
	switch uid := v.(type) {
	case int64:
		return uid, true
	case int:
		return int64(uid), true
	case float64:
		return int64(uid), true
	case string:
		id, err := strconv.ParseInt(uid, 10, 64)
		if err != nil {
			return 0, false
		}
		return id, true
	}
	return 0, false
}

// respondError 统一将 service 层业务错误映射到 HTTP 响应。
// 已知业务错误（*service.BizError）按其 HTTP 状态码返回，并将稳定业务码透出到 Tip 供前端分支；
// 未知错误统一返回 500 且不携带内部错误细节，避免泄露 SQL/路径/AI 原始错误。
func respondError(c *gin.Context, err error) {
	var be *service.BizError
	if errors.As(err, &be) {
		switch be.Status {
		case http.StatusUnauthorized:
			httpresp.Unauthorized(c, be.Msg)
		case http.StatusForbidden:
			httpresp.Forbidden(c, be.Code, be.Msg)
		case http.StatusConflict:
			httpresp.Conflict(c, be.Code, be.Msg)
		case http.StatusNotImplemented:
			httpresp.NotImplemented(c)
		default: // 400 等其余 4xx：tip 透出稳定业务码，msg 为可读描述
			httpresp.BadRequest(c, be.Code, be.Msg)
		}
		return
	}
	httpresp.InternalError(c, "internal error", "")
}

// parseSessionID 解析 session 字符串为 int64。
func parseSessionID(s string) (int64, error) {
	return strconv.ParseInt(s, 10, 64)
}

