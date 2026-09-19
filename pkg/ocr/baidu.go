// Package ocr 封装百度智能云 OCR 客户端。
package ocr

import (
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
	ocrURL   = "https://aip.baidubce.com/rest/2.0/ocr/v1/general_basic"
)

// Baidu 百度智能云 OCR 客户端。
type Baidu struct {
	apiKey    string
	secretKey string
	http      *http.Client

	mu        sync.Mutex
	token     string    // 缓存的 access_token
	expiresAt time.Time // 缓存 token 的过期时间
}

// NewBaidu 创建百度 OCR 客户端。
func NewBaidu(apiKey, secretKey string) *Baidu {
	return &Baidu{
		apiKey:    apiKey,
		secretKey: secretKey,
		http:      &http.Client{Timeout: 30 * time.Second},
	}
}

// apiError 百度 API 业务错误（HTTP 200 但业务码非 0）。
type apiError struct {
	code int
	msg  string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("baidu api error %d: %s", e.code, e.msg)
}

// isTokenError 是否 access_token 失效（110）或过期（111）。
func (e *apiError) isTokenError() bool {
	return e.code == 110 || e.code == 111
}

// Recognize 识别图片（base64 直传）中的文字，返回逐行拼接的文本。
// 当缓存的 token 失效时自动刷新并重试一次。
func (b *Baidu) Recognize(ctx context.Context, image []byte) (string, error) {
	token, err := b.accessToken(ctx)
	if err != nil {
		return "", err
	}
	text, err := b.recognizeOnce(ctx, token, image)
	var ae *apiError
	if errors.As(err, &ae) && ae.isTokenError() {
		b.invalidateToken()
		if token, err = b.accessToken(ctx); err != nil {
			return "", err
		}
		text, err = b.recognizeOnce(ctx, token, image)
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

	resp, err := b.post(ctx, tokenURL, form)
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
func (b *Baidu) recognizeOnce(ctx context.Context, token string, image []byte) (string, error) {
	form := url.Values{}
	form.Set("image", base64.StdEncoding.EncodeToString(image))
	form.Set("detect_direction", "false")
	form.Set("paragraph", "false")
	form.Set("probability", "false")
	form.Set("multidirectional_recognize", "false")

	rawURL := ocrURL + "?access_token=" + url.QueryEscape(token)
	resp, err := b.post(ctx, rawURL, form)
	if err != nil {
		return "", err
	}
	var body struct {
		WordsResultNum int `json:"words_result_num"`
		WordsResult    []struct {
			Words string `json:"words"`
		} `json:"words_result"`
		ErrorCode int    `json:"error_code"`
		ErrorMsg  string `json:"error_msg"`
	}
	if err := json.Unmarshal(resp, &body); err != nil {
		return "", fmt.Errorf("parse ocr response: %w", err)
	}
	if body.ErrorCode != 0 || body.ErrorMsg != "" {
		return "", &apiError{code: body.ErrorCode, msg: body.ErrorMsg}
	}
	lines := make([]string, 0, len(body.WordsResult))
	for _, w := range body.WordsResult {
		lines = append(lines, w.Words)
	}
	return strings.Join(lines, "\n"), nil
}

// invalidateToken 清除缓存的 access_token，强制下次重新获取。
func (b *Baidu) invalidateToken() {
	b.mu.Lock()
	b.token = ""
	b.expiresAt = time.Time{}
	b.mu.Unlock()
}

// post 发送 application/x-www-form-urlencoded 表单请求并返回响应体。
func (b *Baidu) post(ctx context.Context, rawURL string, form url.Values) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := b.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}
