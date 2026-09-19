// Package stt 封装百度智能云语音识别（短语音识别 server_api）客户端。
package stt

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	tokenURL = "https://aip.baidubce.com/oauth/2.0/token"
	asrURL   = "https://vop.baidu.com/server_api"
	// defaultCUID 客户端唯一标识（百度要求必填）。
	defaultCUID = "zhonghuawenhua_backend"
	// defaultRate 采样率（仅 pcm 格式生效，其他格式从文件头读取）。
	defaultRate = 16000
)

// SupportedFormats 百度短语音识别支持的音频格式（key 为资源后缀，value 为提交给百度的 format 参数）。
// 短语音识别标准版/极速版均仅支持 pcm/wav/amr/m4a（16k 或 8k 采样率、16bit 位深、单声道），不支持 mp3；
// 微信小程序 AAC 录音按 m4a 提交。
var SupportedFormats = map[string]string{
	"pcm": "pcm",
	"wav": "wav",
	"amr": "amr",
	"m4a": "m4a",
	"aac": "m4a",
}

// Baidu 百度智能云短语音识别客户端。
type Baidu struct {
	apiKey    string
	secretKey string
	cuid      string
	rate      int
	http      *http.Client

	mu        sync.Mutex
	token     string    // 缓存的 access_token
	expiresAt time.Time // 缓存 token 的过期时间
}

// NewBaidu 创建百度短语音识别客户端。
func NewBaidu(apiKey, secretKey string) *Baidu {
	return &Baidu{
		apiKey:    apiKey,
		secretKey: secretKey,
		cuid:      defaultCUID,
		rate:      defaultRate,
		http:      &http.Client{Timeout: 30 * time.Second},
	}
}

// apiError 百度 API 业务错误（HTTP 200 但 err_no 非 0）。
type apiError struct {
	code int
	msg  string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("baidu asr error %d: %s", e.code, e.msg)
}

// isTokenError 是否 access_token 校验失败（3302）。3301 为音频质量过差，不触发 token 刷新。
func (e *apiError) isTokenError() bool {
	return e.code == 3302
}

// Recognize 识别音频中的语音，format 为音频格式（pcm/wav/amr/m4a），返回识别文本。
// 当缓存的 token 失效时自动刷新并重试一次。
func (b *Baidu) Recognize(ctx context.Context, format string, audio []byte) (string, error) {
	token, err := b.accessToken(ctx)
	if err != nil {
		return "", err
	}
	text, err := b.recognizeOnce(ctx, token, format, audio)
	var ae *apiError
	if errors.As(err, &ae) && ae.isTokenError() {
		b.invalidateToken()
		if token, err = b.accessToken(ctx); err != nil {
			return "", err
		}
		text, err = b.recognizeOnce(ctx, token, format, audio)
	}
	return text, err
}

// accessToken 获取并缓存 access_token（默认有效期 30 天，提前 60s 刷新）。
func (b *Baidu) accessToken(ctx context.Context) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.token != "" && time.Now().Before(b.expiresAt) {
		return b.token, nil
	}
	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", b.apiKey)
	form.Set("client_secret", b.secretKey)

	resp, err := b.postForm(ctx, tokenURL, form)
	if err != nil {
		return "", err
	}
	var body struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		Error       string `json:"error"`
	}
	if err := json.Unmarshal(resp, &body); err != nil {
		return "", fmt.Errorf("parse token response: %w", err)
	}
	if body.AccessToken == "" {
		return "", fmt.Errorf("baidu token failed: %s", body.Error)
	}
	ttl := body.ExpiresIn
	if ttl <= 0 {
		ttl = 2592000 // 默认 30 天
	}
	if ttl > 60 {
		ttl -= 60 // 预留 60s 缓冲，避免边界过期
	}
	b.token = body.AccessToken
	b.expiresAt = time.Now().Add(time.Duration(ttl) * time.Second)
	return b.token, nil
}

// recognizeOnce 使用指定 token 识别一次。
func (b *Baidu) recognizeOnce(ctx context.Context, token, format string, audio []byte) (string, error) {
	payload := map[string]interface{}{
		"format":  format,
		"rate":    b.rate,
		"channel": 1,
		"cuid":    b.cuid,
		"token":   token,
		"speech":  base64.StdEncoding.EncodeToString(audio),
		"len":     len(audio),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal asr payload: %w", err)
	}
	resp, err := b.postJSON(ctx, asrURL, body)
	if err != nil {
		return "", err
	}
	var res struct {
		ErrNo  int      `json:"err_no"`
		ErrMsg string   `json:"err_msg"`
		Result []string `json:"result"`
	}
	if err := json.Unmarshal(resp, &res); err != nil {
		return "", fmt.Errorf("parse asr response: %w", err)
	}
	if res.ErrNo != 0 {
		return "", &apiError{code: res.ErrNo, msg: res.ErrMsg}
	}
	if len(res.Result) == 0 {
		return "", nil
	}
	return res.Result[0], nil
}

// invalidateToken 清除缓存的 access_token，强制下次重新获取。
func (b *Baidu) invalidateToken() {
	b.mu.Lock()
	b.token = ""
	b.expiresAt = time.Time{}
	b.mu.Unlock()
}

// postForm 发送 application/x-www-form-urlencoded 表单请求并返回响应体。
func (b *Baidu) postForm(ctx context.Context, rawURL string, form url.Values) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	return b.do(req)
}

// postJSON 发送 application/json 请求并返回响应体。
func (b *Baidu) postJSON(ctx context.Context, rawURL string, body []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	return b.do(req)
}

// do 发送请求并读取响应体。
func (b *Baidu) do(req *http.Request) ([]byte, error) {
	resp, err := b.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}
