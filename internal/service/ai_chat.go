package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"zhonghuawenhua_backend/internal/api"
	"zhonghuawenhua_backend/internal/config"
	"zhonghuawenhua_backend/internal/model"
	"zhonghuawenhua_backend/pkg/ai"
)

// PostAIChatReq AI 聊天请求。
type PostAIChatReq struct {
	Content      string `json:"content"`
	SessionID    int64  `json:"session_id,omitempty"`
	ClassID      int64  `json:"class_id,omitempty"`      // 可选，关联班级
	NodeID       int64  `json:"node_id,omitempty"`       // 可选，关联节点（用于获取该题目专属提示词）
	SystemPrompt string `json:"system_prompt,omitempty"` // 可选，自定义提示词（优先级最高）
}

// AIService AI 对话业务逻辑。
type AIService struct {
	repos   *Repositories
	llm     ai.LLMClient
	prompts config.PromptsConfig // 提示词配置（从 YAML 加载）
}

// NewAIService 创建 AIService。
func NewAIService(repos *Repositories, llm ai.LLMClient, prompts config.PromptsConfig) *AIService {
	return &AIService{repos: repos, llm: llm, prompts: prompts}
}

// Chat 发送消息并获取 AI 回复。提示词优先级：请求自带 > 节点 prompt_template > 节点类型默认 > 通用默认。
func (s *AIService) Chat(ctx context.Context, userID int64, req *PostAIChatReq) (*ChatResult, error) {
	// 防御性要求：空 content 直接拒绝，避免触发真实 LLM 调用并写入空消息垃圾数据
	if req == nil || strings.TrimSpace(req.Content) == "" {
		return nil, ErrEmptyInput
	}
	sessionID := req.SessionID
	var systemPrompt string

	if sessionID == 0 {
		// 新会话：解析提示词并创建 session
		systemPrompt = s.resolvePrompt(ctx, req)
		newID, err := s.repos.AISession.Create(ctx, &model.AISession{
			UserID:         ptrInt64(userID),
			ClassID:        ptrInt64If(req.ClassID),
			NodeID:         ptrInt64If(req.NodeID),
			Title:          ptrString(truncate(req.Content, 20)),
			PromptTemplate: ptrString(systemPrompt),
		})
		if err != nil {
			return nil, fmt.Errorf("create session: %w", err)
		}
		sessionID = newID
	} else {
		// 已有会话：校验权限 + 使用创建时的提示词
		sess, err := s.repos.AISession.GetByID(ctx, sessionID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, ErrAISessionNotFound
			}
			return nil, fmt.Errorf("get session: %w", err)
		}
		// 归属校验：user_id 为 NULL（异常/遗留数据）同样视为越权，防御性默认拒绝
		if sess.UserID == nil || *sess.UserID != userID {
			return nil, ErrAISessionAccessDenied
		}
		if sess.PromptTemplate != nil {
			systemPrompt = *sess.PromptTemplate
		} else {
			// 会话创建时未保存提示词，重新解析
			systemPrompt = s.resolvePrompt(ctx, req)
		}
	}

	// 加载全部历史消息以计算 seq（新版 schema ai_messages.seq NOT NULL）
	allMsgs, err := s.repos.AIMessage.ListBySessionIDWithRefs(ctx, sessionID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("load history: %w", err)
	}
	nextSeq := len(allMsgs) + 1

	// 保存用户消息（设置 seq）
	if _, err := s.repos.AIMessage.Create(ctx, &model.AIMessage{
		SessionID: sessionID,
		Role:      model.AIMessageRoleUser,
		Content:   req.Content,
		Seq:       nextSeq,
	}); err != nil {
		return nil, fmt.Errorf("save user message: %w", err)
	}

	// 取最近 20 条作为 LLM 上下文窗口
	messages := make([]ai.Message, 0, len(allMsgs)+1)
	start := 0
	if len(allMsgs) > 20 {
		start = len(allMsgs) - 20
	}
	for _, m := range allMsgs[start:] {
		messages = append(messages, ai.Message{Role: string(m.Role), Content: m.Content})
	}
	messages = append(messages, ai.Message{Role: "user", Content: req.Content})

	// 调用 LLM
	resp, err := s.llm.Chat(ctx, &ai.ChatRequest{
		Messages:     messages,
		MaxTokens:    2048,
		Temperature:  0.7,
		SystemPrompt: systemPrompt,
	})
	if err != nil {
		return nil, fmt.Errorf("llm chat: %w", err)
	}

	// 保存助手回复（seq = nextSeq + 1）
	if _, err := s.repos.AIMessage.Create(ctx, &model.AIMessage{
		SessionID:        sessionID,
		Role:             model.AIMessageRoleAssistant,
		Content:          resp.Content,
		ModelName:        ptrString(resp.Provider),
		PromptTokens:     ptrInt(resp.Usage.InputTokens),
		CompletionTokens: ptrInt(resp.Usage.OutputTokens),
		Seq:              nextSeq + 1,
	}); err != nil {
		return nil, fmt.Errorf("save assistant message: %w", err)
	}

	return &ChatResult{
		SessionID: sessionID,
		Messages: api.AIMessages{
			{Role: "user", Content: req.Content},
			{Role: "assistant", Content: resp.Content},
		},
	}, nil
}

// CreateSession 仅创建 AI 会话（不发送消息），返回新会话 ID。
// 用于"创建对话 Session"接口：不要求 content，避免 Chat 因空消息被 ErrEmptyInput 拒绝。
func (s *AIService) CreateSession(ctx context.Context, userID int64, req *PostAIChatReq) (int64, error) {
	systemPrompt := s.resolvePrompt(ctx, req)
	newID, err := s.repos.AISession.Create(ctx, &model.AISession{
		UserID:         ptrInt64(userID),
		ClassID:        ptrInt64If(req.ClassID),
		NodeID:         ptrInt64If(req.NodeID),
		Title:          ptrString("新对话"),
		PromptTemplate: ptrString(systemPrompt),
	})
	if err != nil {
		return 0, fmt.Errorf("create session: %w", err)
	}
	return newID, nil
}

// resolvePrompt 按优先级解析 system prompt：
// 1. 请求自带 system_prompt（最高优先级）
// 2. 节点的 node_contents.params_json.system_prompt 字段
// 3. 节点的 prompt_template 字段
// 4. 根据 node_type 返回类型默认提示词
// 5. 通用中华文化教学助手提示词（兜底）
func (s *AIService) resolvePrompt(ctx context.Context, req *PostAIChatReq) string {
	// 1. 请求自带
	if req.SystemPrompt != "" {
		return req.SystemPrompt
	}

	// 2-4: 需要节点信息
	if req.NodeID > 0 {
		return s.resolveNodePrompt(ctx, req.NodeID)
	}

	// 5: 默认
	return s.defaultPrompt()
}

// resolveNodePrompt 从节点获取专属提示词。
func (s *AIService) resolveNodePrompt(ctx context.Context, nodeID int64) string {
	// 尝试从 node_contents 读取
	nc, err := s.repos.NodeContent.GetByNodeID(ctx, nodeID)
	if err == nil && nc != nil && len(nc.ParamsJSON) > 0 {
		if sp := extractSystemPrompt(nc.ParamsJSON); sp != "" {
			return sp
		}
	}

	// 尝试从 class_node 读取 node_type，按类型返回默认提示词
	node, err := s.repos.ClassNode.GetByID(ctx, nodeID)
	if err == nil && node != nil {
		return s.defaultPromptForType(string(node.NodeType))
	}

	return s.defaultPrompt()
}

// extractSystemPrompt 从 params_json 的 JSONB 中提取 system_prompt 字段。
func extractSystemPrompt(raw json.RawMessage) string {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	sp, ok := m["system_prompt"]
	if !ok {
		return ""
	}
	var s string
	if err := json.Unmarshal(sp, &s); err != nil {
		return ""
	}
	return s
}

// defaultPromptForType 根据节点类型返回默认提示词（从配置读取）。
func (s *AIService) defaultPromptForType(nodeType string) string {
	if p, ok := s.prompts.Types[nodeType]; ok && p != "" {
		return p
	}
	return s.defaultPrompt()
}

// defaultPrompt 返回通用默认提示词（从配置读取）。
func (s *AIService) defaultPrompt() string {
	if s.prompts.Default != "" {
		return s.prompts.Default
	}
	return "你是一位中华文化教学助手，擅长以生动有趣的方式向中小学生讲解中国传统文化知识。回答要简洁、准确、适合学生理解。"
}

// GetMessages 获取会话历史消息。
func (s *AIService) GetMessages(ctx context.Context, userID, sessionID int64) (*ChatResult, error) {
	sess, err := s.repos.AISession.GetByID(ctx, sessionID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAISessionNotFound
		}
		return nil, fmt.Errorf("get session: %w", err)
	}
	// 归属校验：user_id 为 NULL（异常/遗留数据）同样视为越权，防御性默认拒绝
	if sess.UserID == nil || *sess.UserID != userID {
		return nil, ErrAISessionAccessDenied
	}

	msgs, err := s.repos.AIMessage.ListBySessionID(ctx, sessionID, 100, 0)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &ChatResult{SessionID: sessionID, Messages: api.AIMessages{}}, nil
		}
		return nil, fmt.Errorf("list messages: %w", err)
	}

	result := make(api.AIMessages, len(msgs))
	for i, m := range msgs {
		result[i] = struct {
			Content    string                   `json:"content"`
			References *[]api.AISearchReference `json:"references,omitempty"`
			Role       string                   `json:"role"`
		}{
			Role:    string(m.Role),
			Content: m.Content,
		}
	}
	return &ChatResult{SessionID: sessionID, Messages: result}, nil
}

// ChatResult AI 聊天结果。
type ChatResult struct {
	SessionID int64          `json:"sessionId"`
	Messages  api.AIMessages `json:"messages"`
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func ptrInt64(v int64) *int64 { return &v }
func ptrInt64If(v int64) *int64 {
	if v == 0 {
		return nil
	}
	return &v
}
func ptrString(v string) *string { return &v }
func ptrInt(v int) *int          { return &v }
