package service

import (
	"context"
	"encoding/json"
	"fmt"

	"zhonghuawenhua_backend/internal/api"
)

// --- 赵州桥 params_json 结构 ---

// ZhaoZhouQiaoParams 赵州桥节点的 params_json 结构。
//
// 示例:
//
//	{
//	  "title": "宣传有法：学习《赵州桥》的表达方法",
//	  "introVideo": {"autoPlay": true, "url": "/assets/zhaozhouqiao_intro.mp4"},
//	  "introBubbleText": "亲爱的同学...",
//	  "cards": [
//	    {"id": "c1", "title": "...", "description": "...", "voiceOnly": false,
//	     "input": {"placeholder": "...", "length": {"min": 10, "max": 200}},
//	     "referenceAnswer": "...", "maxErrors": 3,
//	     "assets": [{"type": "image", "src": "/assets/p1.png"}]}
//	  ]
//	}
type ZhaoZhouQiaoParams struct {
	Title           string              `json:"title"`
	IntroVideo      *api.IntroVideo     `json:"introVideo,omitempty"`
	IntroBubbleText *string             `json:"introBubbleText,omitempty"`
	Cards           []ZhaoZhouQiaoCardDef `json:"cards"`
}

// ZhaoZhouQiaoCardDef 赵州桥问题卡定义。
type ZhaoZhouQiaoCardDef struct {
	ID              string                `json:"id"`
	Title           string                `json:"title"`
	Description     string                `json:"description,omitempty"`
	VoiceOnly       bool                  `json:"voiceOnly,omitempty"`
	Input           *ZhaoZhouQiaoInputDef `json:"input,omitempty"`
	ReferenceAnswer string                `json:"referenceAnswer,omitempty"`
	MaxErrors       int                   `json:"maxErrors,omitempty"`
	Assets          []ZhaoZhouQiaoAssetDef `json:"assets,omitempty"`
}

// ZhaoZhouQiaoAssetDef 额外资源定义。
type ZhaoZhouQiaoAssetDef struct {
	Src  string `json:"src"`
	Type string `json:"type"`
}

// ZhaoZhouQiaoInputDef 输入框配置。
type ZhaoZhouQiaoInputDef struct {
	Placeholder string `json:"placeholder,omitempty"`
	MinLength   int    `json:"min,omitempty"`
	MaxLength   int    `json:"max,omitempty"`
}

// loadZhaoZhouQiaoParams 加载赵州桥节点参数。
func loadZhaoZhouQiaoParams(ctx context.Context, repos *Repositories, nodeID int64) (*ZhaoZhouQiaoParams, error) {
	var p ZhaoZhouQiaoParams
	if err := loadNodeParams(ctx, repos, nodeID, &p); err != nil {
		return nil, err
	}
	if len(p.Cards) == 0 {
		return nil, ErrNodeContentNotConfigured
	}
	for i := range p.Cards {
		if p.Cards[i].MaxErrors == 0 {
			p.Cards[i].MaxErrors = 3
		}
	}
	return &p, nil
}

// FindCard 按 ID 查找问题卡定义，未找到返回 nil。
func (p *ZhaoZhouQiaoParams) FindCard(cardID string) *ZhaoZhouQiaoCardDef {
	for i := range p.Cards {
		if p.Cards[i].ID == cardID {
			return &p.Cards[i]
		}
	}
	return nil
}

// --- 赵州桥 NodeStateService 方法 ---

// ZhaoZhouQiaoParamsDTO 赵州桥 params 接口响应数据。
// OpenAPI 中该接口响应为 inline 对象（未命名为 schema）。
type ZhaoZhouQiaoParamsDTO struct {
	Title           string          `json:"title"`
	IntroVideo      *api.IntroVideo `json:"introVideo,omitempty"`
	IntroBubbleText *string         `json:"introBubbleText,omitempty"`
}

// GetZhaoZhouQiaoParams 获取赵州桥节点静态参数。
// 返回页面固定信息（标题、视频、气泡文字），不含问题卡定义。
// 问题卡定义在 GetZhaoZhouQiaoState 中返回（每张卡含 submitLogs）。
func (s *NodeStateService) GetZhaoZhouQiaoParams(ctx context.Context, nodeID int64) (*ZhaoZhouQiaoParamsDTO, error) {
	params, err := loadZhaoZhouQiaoParams(ctx, s.repos, nodeID)
	if err != nil {
		return nil, err
	}
	return &ZhaoZhouQiaoParamsDTO{
		Title:           params.Title,
		IntroVideo:      params.IntroVideo,
		IntroBubbleText: params.IntroBubbleText,
	}, nil
}

// GetZhaoZhouQiaoState 获取赵州桥节点状态。
// 返回每张问题卡的固定定义 + 该用户该节点的全部 submitLogs（按 cardId 分组）。
func (s *NodeStateService) GetZhaoZhouQiaoState(ctx context.Context, classID, nodeID, userID int64) (*api.ZhaoZhouQiaoState, error) {
	if err := s.repos.EnsureClassMember(ctx, classID, userID); err != nil {
		return nil, err
	}
	params, err := loadZhaoZhouQiaoParams(ctx, s.repos, nodeID)
	if err != nil {
		return nil, err
	}

	// 拉取该用户该节点全部提交记录
	submissions, err := s.repos.NodeSubmission.ListByUserNode(ctx, userID, nodeID, 200, 0)
	if err != nil {
		return nil, fmt.Errorf("list submissions: %w", err)
	}

	// 按 cardId 分组组装 submitLogs
	logsByCard := make(map[string][]api.ZhaoZhouQiaoSumbitResp, len(params.Cards))
	for _, sub := range submissions {
		if sub.QuestionID == nil {
			continue
		}
		// payload_json 存 cardId（与 SubmitRequest.CardID 对齐）
		var p map[string]string
		if len(sub.PayloadJSON) > 0 {
			_ = json.Unmarshal(sub.PayloadJSON, &p)
		}
		cardID := p["cardId"]
		if cardID == "" {
			// 兼容老数据：若 payload 没 cardId，用 QuestionID
			cardID = *sub.QuestionID
		}
		logsByCard[cardID] = append(logsByCard[cardID], sub.ToZhaoZhouQiaoResp())
	}

	// 组装响应：每张卡的定义 + submitLogs
	state := &api.ZhaoZhouQiaoState{}
	for _, c := range params.Cards {
		card := buildZhaoZhouQiaoCardDTO(c)
		if logs, ok := logsByCard[c.ID]; ok {
			card.SubmitLogs = logs
		} else {
			card.SubmitLogs = []api.ZhaoZhouQiaoSumbitResp{}
		}
		state.Cards = append(state.Cards, card)
	}
	return state, nil
}

// zhaoZhouQiaoAssetDTO 与 api.ZhaoZhouQiaoState.Cards.Assets 元素类型（内联匿名结构体）保持一致。
type zhaoZhouQiaoAssetDTO = struct {
	Src  string                               `json:"src"`
	Type api.ZhaoZhouQiaoStateCardsAssetsType `json:"type"`
}

// zhaoZhouQiaoCardDTO 与 api.ZhaoZhouQiaoState.Cards 元素类型（内联匿名结构体）保持一致。
type zhaoZhouQiaoCardDTO = struct {
	Assets      *[]zhaoZhouQiaoAssetDTO `json:"assets,omitempty"`
	Description *string                 `json:"description,omitempty"`
	ID          string                  `json:"id"`
	Input       *struct {
		Length      interface{} `json:"length,omitempty"`
		Placeholder *string     `json:"placeholder,omitempty"`
	} `json:"input,omitempty"`
	SubmitLogs []api.ZhaoZhouQiaoSumbitResp `json:"submitLogs"`
	Title      string                       `json:"title"`
	VoiceOnly  *bool                        `json:"voiceOnly,omitempty"`
}

// buildZhaoZhouQiaoCardDTO 将内部问题卡定义转为 api.ZhaoZhouQiaoState.Cards 元素。
func buildZhaoZhouQiaoCardDTO(c ZhaoZhouQiaoCardDef) zhaoZhouQiaoCardDTO {
	card := zhaoZhouQiaoCardDTO{
		ID:    c.ID,
		Title: c.Title,
	}
	if c.Description != "" {
		card.Description = &c.Description
	}
	if c.VoiceOnly {
		t := true
		card.VoiceOnly = &t
	}
	if c.Input != nil {
		card.Input = &struct {
			Length      interface{} `json:"length,omitempty"`
			Placeholder *string     `json:"placeholder,omitempty"`
		}{
			Placeholder: strPtrIfSet(c.Input.Placeholder),
			Length: struct {
				Min int `json:"min"`
				Max int `json:"max"`
			}{Min: c.Input.MinLength, Max: c.Input.MaxLength},
		}
	}
	if len(c.Assets) > 0 {
		assets := make([]zhaoZhouQiaoAssetDTO, len(c.Assets))
		for i, a := range c.Assets {
			assets[i] = zhaoZhouQiaoAssetDTO{
				Src:  a.Src,
				Type: api.ZhaoZhouQiaoStateCardsAssetsType(a.Type),
			}
		}
		card.Assets = &assets
	}
	return card
}
