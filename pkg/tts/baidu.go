// Package tts 封装百度智能云语音合成（text2audio）客户端。
package tts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	tokenURL = "https://aip.baidubce.com/oauth/2.0/token"
	ttsURL   = "https://tsn.baidu.com/text2audio"
	// defaultCUID 客户端唯一标识（百度要求必填）。
	defaultCUID = "zhonghuawenhua_backend"
	// defaultPer 发音人：0 普通女声 / 1 普通男声（可调整）。
	defaultPer = 1
	// defaultSpd 语速（0-9，默认 5）。
	defaultSpd = 5
	// defaultPit 音调（0-9，默认 5）。
	defaultPit = 5
	// defaultVol 音量（0-15，默认 5）。
	defaultVol = 5
	// defaultAue 音频格式：3=mp3，4=pcm。
	defaultAue = 3
	// maxTextBytes 普通发音人单次合成的最大文本字节数（百度限制 1024 字节）。
	maxTextBytes = 1024
)

// Baidu 百度智能云语音合成（text2audio）客户端。
type Baidu struct {
	apiKey    string
	secretKey string
	cuid      string
	http      *http.Client

	mu        sync.Mutex
	token     string    // 缓存的 access_token
	expiresAt time.Time // 缓存 token 的过期时间
}

// NewBaidu 创建百度语音合成客户端。
func NewBaidu(apiKey, secretKey string) *Baidu {
	return &Baidu{
		apiKey:    apiKey,
		secretKey: secretKey,
		cuid:      defaultCUID,
		http:      &http.Client{Timeout: 30 * time.Second},
	}
}

// apiError 百度 API 业务错误（HTTP 200 但 err_no 非 0）。
type apiError struct {
	code int
	msg  string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("baidu tts error %d: %s", e.code, e.msg)
}

// isTokenError 是否 access_token 无效（3301）或过期（3302）。
func (e *apiError) isTokenError() bool {
	return e.code == 3301 || e.code == 3302
}

// Synthesize 将文本合成为音频（mp3），返回音频字节。
// 当缓存的 token 失效时自动刷新并重试一次。
func (b *Baidu) Synthesize(ctx context.Context, text string) ([]byte, error) {
	if text == "" {
		return nil, errors.New("tts text must not be empty")
	}
	if n := len([]byte(text)); n > maxTextBytes {
		return nil, fmt.Errorf("tts text too long: %d bytes > %d", n, maxTextBytes)
	}
	token, err := b.accessToken(ctx)
	if err != nil {
		return nil, err
	}
	audio, err := b.synthesizeOnce(ctx, token, text)
	var ae *apiError
	if errors.As(err, &ae) && ae.isTokenError() {
		b.invalidateToken()
		if token, err = b.accessToken(ctx); err != nil {
			return nil, err
		}
		audio, err = b.synthesizeOnce(ctx, token, text)
	}
	return audio, err
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

// synthesizeOnce 使用指定 token 合成一次音频。
// 合成成功时响应为音频二进制；失败时百度返回 JSON 错误（err_no/err_msg）。
func (b *Baidu) synthesizeOnce(ctx context.Context, token, text string) ([]byte, error) {
	form := url.Values{}
	form.Set("tex", text)
	form.Set("tok", token)
	form.Set("cuid", b.cuid)
	form.Set("ctp", "1")
	form.Set("lan", "zh")
	form.Set("spd", strconv.Itoa(defaultSpd))
	form.Set("pit", strconv.Itoa(defaultPit))
	form.Set("vol", strconv.Itoa(defaultVol))
	form.Set("per", strconv.Itoa(defaultPer))
	form.Set("aue", strconv.Itoa(defaultAue))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ttsURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := b.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	// 百度以 JSON 形式返回业务错误（无论 HTTP 状态码是否 200），音频内容以 { 开头的概率极低，据此区分
	if strings.HasPrefix(strings.TrimSpace(string(body)), "{") {
		var e struct {
			ErrNo  int    `json:"err_no"`
			ErrMsg string `json:"err_msg"`
		}
		if err := json.Unmarshal(body, &e); err == nil && e.ErrNo != 0 {
			return nil, &apiError{code: e.ErrNo, msg: e.ErrMsg}
		}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("baidu tts http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
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
