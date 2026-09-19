// Package ai 提供 LLM 抽象层，支持多供应商。
package ai

import (
	"context"
)

// Message 对话消息（通用结构）。
type Message struct {
	Role    string `json:"role"`    // user / assistant / system
	Content string `json:"content"`
}

// ChatRequest 聊天请求。
type ChatRequest struct {
	Messages   []Message `json:"messages"`
	Model      string    `json:"model"`
	MaxTokens  int       `json:"max_tokens,omitempty"`
	Temperature float64 `json:"temperature,omitempty"`
	SystemPrompt string `json:"system_prompt,omitempty"` // 仅 Anthropic 使用
}

// ChatResponse 聊天响应。
type ChatResponse struct {
	Content     string `json:"content"`
	Usage       Usage  `json:"usage"`
	Provider    string `json:"provider"`
	RawResponse []byte `json:"-"`
}

// Usage token 用量。
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// LLMClient LLM 客户端抽象接口。
// 支持 OpenAI 兼容接口（含通义千问等）和 Anthropic Claude。
type LLMClient interface {
	// Chat 发送聊天请求并返回响应。
	Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error)
	// ProviderName 返回供应商标识。
	ProviderName() string
}
