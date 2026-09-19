package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// AnthropicConfig Anthropic Claude 客户端配置。
type AnthropicConfig struct {
	APIKey  string
	BaseURL string // 留空则使用 https://api.anthropic.com
	Model   string // claude-3-5-haiku-20241022 等
}

// NewAnthropicClient 创建 Anthropic Claude 客户端。
func NewAnthropicClient(cfg AnthropicConfig) LLMClient {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.anthropic.com"
	}
	// 防御性要求：去掉 BaseURL 末尾斜杠，避免拼接 /v1/messages 时出现双斜杠
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	return &anthropicClient{cfg: cfg, httpCli: &http.Client{Timeout: 60 * time.Second}}
}

type anthropicClient struct {
	cfg    AnthropicConfig
	httpCli *http.Client
}

func (c *anthropicClient) ProviderName() string { return "anthropic" }

// anthropicReq / anthropicResp 对应 Anthropic Messages API。
type anthropicReq struct {
	Model       string            `json:"model"`
	MaxTokens   int               `json:"max_tokens"`
	System      string            `json:"system,omitempty"`
	Messages    []anthropicMsg    `json:"messages"`
	Temperature float64           `json:"temperature,omitempty"`
}
type anthropicMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
type anthropicResp struct {
	Content []struct {
		Type string `json:"type"` // text
		Text string `json:"text"`
	} `json:"content"`
 Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
 } `json:"usage"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (c *anthropicClient) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	messages := make([]anthropicMsg, len(req.Messages))
	for i, m := range req.Messages {
		messages[i] = anthropicMsg{Role: m.Role, Content: m.Content}
	}

	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 1024
	}

	// req.Model 为空时回退到客户端配置的模型
	model := req.Model
	if model == "" {
		model = c.cfg.Model
	}

	body, _ := json.Marshal(anthropicReq{
		Model:       model,
		MaxTokens:   maxTokens,
		System:      req.SystemPrompt,
		Messages:    messages,
		Temperature: req.Temperature,
	})
	if body == nil {
		body = []byte("{}")
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", c.cfg.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.httpCli.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	// 防御性要求：先校验 HTTP 状态码，避免上游 4xx/5xx 被静默当作成功
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("api http %d: %s", resp.StatusCode, truncateBody(respBody))
	}

	var aresp anthropicResp
	if err := json.Unmarshal(respBody, &aresp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}
	if aresp.Error != nil {
		return nil, fmt.Errorf("api error [%s]: %s", aresp.Error.Type, aresp.Error.Message)
	}

	content := ""
	for _, block := range aresp.Content {
		if block.Type == "text" {
			content = block.Text
			break
		}
	}

	return &ChatResponse{
		Content:     content,
		Usage:       Usage{InputTokens: aresp.Usage.InputTokens, OutputTokens: aresp.Usage.OutputTokens},
		Provider:    "anthropic",
		RawResponse: respBody,
	}, nil
}
