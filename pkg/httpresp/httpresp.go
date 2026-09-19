// Package httpresp 封装 api.gen.go 中的 Base 响应类型。
package httpresp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"zhonghuawenhua_backend/internal/api"
)

// toMap 将 data 转换为 map[string]interface{}。
// 支持 nil、map[string]interface{} 以及任意 struct（通过 json 序列化反序列化转换）。
// 防御性要求：① 序列化/反序列化错误必须返回 error，禁止吞错后返回假数据；
// ② 使用 UseNumber 保留 int64 精度，避免 ID/时间戳经 JSON 往返丢失精度。
func toMap(data interface{}) (map[string]interface{}, error) {
	if data == nil {
		return nil, nil
	}
	if m, ok := data.(map[string]interface{}); ok {
		return m, nil
	}
	b, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("marshal response data: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var m map[string]interface{}
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("decode response data: %w", err)
	}
	return m, nil
}

func ptrMsg(msg string) *string {
	if msg == "" {
		return nil
	}
	return &msg
}

// OK 返回成功响应（HTTP 200）。data 无法序列化时返回 500，避免 200 假成功。
func OK(c *gin.Context, data interface{}) {
	m, err := toMap(data)
	if err != nil {
		InternalError(c, "response marshal failed", err.Error())
		return
	}
	c.JSON(http.StatusOK, api.Base200Resp{
		Code: http.StatusOK,
		Data: m,
		Msg:  ptrMsg("ok"),
	})
}

// BadRequest 返回客户端错误响应（HTTP 400）。tip 必填。
func BadRequest(c *gin.Context, tip, msg string) {
	c.JSON(http.StatusBadRequest, api.Base400Resp{
		Code: http.StatusBadRequest,
		Tip:  tip,
		Msg:  ptrMsg(msg),
		Data: nil,
	})
}

// Unauthorized 返回未认证响应（HTTP 401），tip 为空字符串。
func Unauthorized(c *gin.Context, msg string) {
	c.JSON(http.StatusUnauthorized, api.Base401Resp{
		Code: http.StatusUnauthorized,
		Tip:  "",
		Msg:  ptrMsg(msg),
		Data: nil,
	})
}

// Forbidden 返回禁止访问响应（HTTP 403），tip 透出稳定业务码。
func Forbidden(c *gin.Context, tip, msg string) {
	c.JSON(http.StatusForbidden, api.Base400Resp{
		Code: http.StatusForbidden,
		Tip:  tip,
		Msg:  ptrMsg(msg),
		Data: nil,
	})
}

// Conflict 返回冲突响应（HTTP 409），tip 透出稳定业务码。
func Conflict(c *gin.Context, tip, msg string) {
	c.JSON(http.StatusConflict, api.Base400Resp{
		Code: http.StatusConflict,
		Tip:  tip,
		Msg:  ptrMsg(msg),
		Data: nil,
	})
}

// InternalError 返回服务器错误响应（HTTP 500）。tip 必填。
func InternalError(c *gin.Context, tip, msg string) {
	c.JSON(http.StatusInternalServerError, api.Base500Resp{
		Code: http.StatusInternalServerError,
		Tip:  tip,
		Msg:  ptrMsg(msg),
		Data: nil,
	})
}

// NotImplemented 统一返回 501 占位。
func NotImplemented(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, api.Base500Resp{
		Code: http.StatusNotImplemented,
		Tip:  "not implemented",
		Msg:  ptrMsg("not implemented"),
		Data: nil,
	})
}
