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

// OpenAIConfig OpenAI 兼容客户端配置。
type OpenAIConfig struct {
	APIKey  string
	BaseURL string // 留空则使用 https://api.openai.com/v1
	Model   string
}

// NewOpenAIClient 创建 OpenAI 兼容客户端。
// 支持 OpenAI、通义千问（dashscope）、DeepSeek 等 OpenAI-compatible API。
func NewOpenAIClient(cfg OpenAIConfig) LLMClient {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.openai.com/v1"
	}
	// 去掉末尾斜杠，避免拼接出 //chat/completions 双斜杠路径
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	return &openAIClient{cfg: cfg, httpCli: &http.Client{Timeout: 60 * time.Second}}
}

type openAIClient struct {
	cfg     OpenAIConfig
	httpCli *http.Client
}

func (c *openAIClient) ProviderName() string { return "openai" }

// openAIReq / openAIResp 对应 OpenAI API 的请求/响应结构。
type openAIReq struct {
	Model       string      `json:"model"`
	Messages    []openAIMsg `json:"messages"`
	MaxTokens   int         `json:"max_tokens,omitempty"`
	Temperature float64     `json:"temperature,omitempty"`
}
type openAIMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
type openAIResp struct {
	Choices []struct {
		Message struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"` // 推理模型（如 deepseek-v4-flash）输出走该字段
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

func (c *openAIClient) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	messages := make([]openAIMsg, 0, len(req.Messages)+1)
	if req.SystemPrompt != "" {
		messages = append(messages, openAIMsg{Role: "system", Content: req.SystemPrompt})
	}
	for _, m := range req.Messages {
		messages = append(messages, openAIMsg{Role: m.Role, Content: m.Content})
	}

	// req.Model 为空时回退到客户端配置的模型
	model := req.Model
	if model == "" {
		model = c.cfg.Model
	}

	body, _ := json.Marshal(openAIReq{
		Model:       model,
		Messages:    messages,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
	})
	if body == nil {
		body = []byte("{}")
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)

	resp, err := c.httpCli.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	// 非 2xx 状态码：直接给出状态码与响应体，便于排查（URL、模型、鉴权等问题）
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("api http %d: %s", resp.StatusCode, truncateBody(respBody))
	}

	var oresp openAIResp
	if err := json.Unmarshal(respBody, &oresp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}
	if oresp.Error != nil {
		return nil, fmt.Errorf("api error [%s]: %s", oresp.Error.Type, oresp.Error.Message)
	}
	if len(oresp.Choices) == 0 {
		return nil, fmt.Errorf("empty choices in response")
	}

	content := oresp.Choices[0].Message.Content
	// 推理模型（deepseek-v4-flash 等）content 为空、实际输出在 reasoning_content，
	// 回退到 reasoning_content，否则调用方会拿到空内容导致后续 JSON 解析失败
	if content == "" {
		content = oresp.Choices[0].Message.ReasoningContent
	}

	return &ChatResponse{
		Content:     content,
		Usage:       Usage{InputTokens: oresp.Usage.PromptTokens, OutputTokens: oresp.Usage.CompletionTokens},
		Provider:    "openai",
		RawResponse: respBody,
	}, nil
}

// truncateBody 截断响应体用于错误提示，避免过长或非文本内容刷屏。
func truncateBody(b []byte) string {
	s := string(b)
	if len(s) > 300 {
		s = s[:300] + "..."
	}
	return s
}
