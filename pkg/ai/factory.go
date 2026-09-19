package ai

import (
	"fmt"

	"zhonghuawenhua_backend/internal/config"
)

// NewClient 根据配置创建对应的 LLM 客户端。
// provider: "openai" | "anthropic" | 其他（走 OpenAI 兼容接口）。
func NewClient(cfg config.AIConfig) (LLMClient, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("ai.api_key is required")
	}
	switch cfg.Provider {
	case "anthropic":
		return NewAnthropicClient(AnthropicConfig{
			APIKey:  cfg.APIKey,
			BaseURL: cfg.BaseURL,
			Model:   pickModel(cfg.Model, "claude-3-5-haiku-20241022"),
		}), nil
	default:
		// openai / qwen / custom — 统一走 OpenAI 兼容接口
		return NewOpenAIClient(OpenAIConfig{
			APIKey:  cfg.APIKey,
			BaseURL: cfg.BaseURL,
			Model:   pickModel(cfg.Model, "gpt-4o-mini"),
		}), nil
	}
}

func pickModel(cfg, fallback string) string {
	if cfg != "" {
		return cfg
	}
	return fallback
}
