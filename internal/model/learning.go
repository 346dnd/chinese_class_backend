package model

import (
	"encoding/json"
	"strconv"
	"time"

	"zhonghuawenhua_backend/internal/api"
)

// NodeProgress 学生学习进度缓存表行（对应 node_progress 表）。
// UNIQUE(node_id, user_id) - node_id 全局唯一，class_id 仅用于归属查询。
type NodeProgress struct {
	ID               int64           `db:"id" json:"id"`
	NodeID           int64           `db:"node_id" json:"nodeId"`
	UserID           int64           `db:"user_id" json:"userId"`
	ClassID          int64           `db:"class_id" json:"classId"`
	Completed        bool            `db:"completed" json:"completed"`
	AttemptCount     int             `db:"attempt_count" json:"attemptCount"`
	ErrorCount       int             `db:"error_count" json:"errorCount"`
	Revealed         bool            `db:"revealed" json:"revealed"`
	DraftJSON        json.RawMessage `db:"draft_json" json:"draftJson"`
	LastSubmitID     *int64          `db:"last_submit_id" json:"lastSubmitId,omitempty"`
	Duration         int             `db:"duration" json:"duration"`
	AIEvaluationJSON json.RawMessage `db:"ai_evaluation_json" json:"aiEvaluationJson,omitempty"` // AI 综合评价结果（节点完成后异步生成缓存）
	CreatedAt        time.Time       `db:"created_at" json:"createdAt"`
	UpdatedAt        time.Time       `db:"updated_at" json:"updatedAt"`
}

// NodeSubmission 节点提交明细表行（对应 node_submissions 表）。
// 每次 submit 都 INSERT 新行，保留全部历史（含错误、重试、get-answer）。
type NodeSubmission struct {
	ID              int64           `db:"id" json:"id"`
	Code            string          `db:"code" json:"code"`
	ClassID         int64           `db:"class_id" json:"classId"`
	NodeID          int64           `db:"node_id" json:"nodeId"`
	UserID          int64           `db:"user_id" json:"userId"`
	NodeType        NodeType        `db:"node_type" json:"nodeType"`
	QuestionID      *string         `db:"question_id" json:"questionId,omitempty"`
	SubmitType      string          `db:"submit_type" json:"submitType"`
	PayloadJSON     json.RawMessage `db:"payload_json" json:"payloadJson"`
	ResultJSON      json.RawMessage `db:"result_json" json:"resultJson,omitempty"`
	IsProcessing    bool            `db:"is_processing" json:"isProcessing"`
	IsPassed        *bool           `db:"is_passed" json:"isPassed,omitempty"`
	IsCompleted     *bool           `db:"is_completed" json:"isCompleted,omitempty"`
	ErrorCount      int             `db:"error_count" json:"errorCount"`
	Revealed        bool            `db:"revealed" json:"revealed"`
	Feedback        *string         `db:"feedback" json:"feedback,omitempty"`
	ReferenceAnswer *string         `db:"reference_answer" json:"referenceAnswer,omitempty"`
	AudioResourceID *int64          `db:"audio_resource_id" json:"audioResourceId,omitempty"`
	Duration        *int            `db:"duration" json:"duration,omitempty"`
	PointsEarned    int             `db:"points_earned" json:"pointsEarned"`
	SubmittedAt     time.Time       `db:"submitted_at" json:"submittedAt"`
	CreatedAt       time.Time       `db:"created_at" json:"createdAt"`
	UpdatedAt       time.Time       `db:"updated_at" json:"updatedAt"`
}

// SubmitIDStr 返回提交 ID 的字符串形式，优先使用 Code，否则使用 ID。
func (s *NodeSubmission) SubmitIDStr() string {
	if s.Code != "" {
		return s.Code
	}
	return strconv.FormatInt(s.ID, 10)
}

// strPtrEmpty 返回字符串指针，空串返回 nil。
func strPtrEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// StrPtrEmpty 返回字符串指针，空串返回 nil（导出版本，供 service 包使用）。
func StrPtrEmpty(s string) *string {
	return strPtrEmpty(s)
}

// ToThoughtResp 转换为「写想法」提交响应 DTO（新版 api.WriteThoughtSubmitResult）。
// 新版字段：ID / QuestionID / IsPassed / IsCompleted / IsProcessing / Feedback / ReferenceAnswer /
// OfferReferenceAnswer / SubmittedText / Type。
func (s *NodeSubmission) ToThoughtResp() api.WriteThoughtSubmitResult {
	resp := api.WriteThoughtSubmitResult{
		ID:           s.SubmitIDStr(),
		IsProcessing: s.IsProcessing,
		Type:         api.WriteThoughtSubmitResultTypeSubmit,
	}
	if s.QuestionID != nil {
		resp.QuestionID = *s.QuestionID
	}
	// IsPassed / IsCompleted 直接传递指针
	resp.IsPassed = s.IsPassed
	resp.IsCompleted = s.IsCompleted
	// Feedback / ReferenceAnswer 直接传递指针
	resp.Feedback = s.Feedback
	resp.ReferenceAnswer = s.ReferenceAnswer
	// SubmittedText 从 payload_json 提取
	if len(s.PayloadJSON) > 0 {
		var p map[string]string
		if json.Unmarshal(s.PayloadJSON, &p) == nil {
			if text, ok := p["input"]; ok {
				resp.SubmittedText = &text
			}
		}
	}
	// Type 按 submit_type 设置
	if s.SubmitType == "get-answer" {
		resp.Type = api.WriteThoughtSubmitResultTypeGetAnswer
	}
	// OfferReferenceAnswer：当 Revealed=true 时表示已解锁参考答案，前端可显示获取答案按钮
	if s.Revealed {
		t := true
		resp.OfferReferenceAnswer = &t
	}
	return resp
}

// ToInitialInsightResp 转换为「初步感悟」提交响应 DTO（api.InitialImpressionsSubmitResult）。
func (s *NodeSubmission) ToInitialInsightResp(blankID string) api.InitialImpressionsSubmitResult {
	resp := api.InitialImpressionsSubmitResult{
		ID:            s.SubmitIDStr(),
		IsProcessing:  s.IsProcessing,
		BlankID:       blankID,
		SubmittedText: "",
	}
	if s.QuestionID != nil {
		resp.QuestionID = *s.QuestionID
	}
	resp.IsPassed = s.IsPassed
	resp.IsCompleted = s.IsCompleted
	resp.Feedback = s.Feedback
	resp.ReferenceAnswer = s.ReferenceAnswer
	// SubmittedText 从 payload_json 提取
	if len(s.PayloadJSON) > 0 {
		var p map[string]string
		if json.Unmarshal(s.PayloadJSON, &p) == nil {
			if text, ok := p["input"]; ok {
				resp.SubmittedText = text
			}
		}
	}
	return resp
}

// ptrStrValue 返回字符串指针的值，nil 时返回空串。
func ptrStrValue(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// ToZhaoZhouQiaoResp 转换为「赵州桥」提交响应 DTO（api.ZhaoZhouQiaoSumbitResp）。
// payload_json 存 cardId / input / audio.id。
func (s *NodeSubmission) ToZhaoZhouQiaoResp() api.ZhaoZhouQiaoSumbitResp {
	resp := api.ZhaoZhouQiaoSumbitResp{
		ID:           s.SubmitIDStr(),
		IsProcessing: s.IsProcessing,
		Type:         api.ZhaoZhouQiaoSumbitRespTypeSubmit,
	}
	// CardID 从 QuestionID 或 payload.cardId 取
	if s.QuestionID != nil {
		resp.CardID = *s.QuestionID
	}
	if len(s.PayloadJSON) > 0 {
		var p struct {
			CardID string `json:"cardId"`
			Input  string `json:"input"`
			Text   string `json:"text"`
			Audio  struct {
				ID string `json:"id"`
			} `json:"audio"`
		}
		if json.Unmarshal(s.PayloadJSON, &p) == nil {
			if p.CardID != "" {
				resp.CardID = p.CardID
			}
			// 优先 text，其次 input
			if p.Text != "" {
				resp.SubmittedText = &p.Text
			} else if p.Input != "" {
				resp.SubmittedText = &p.Input
			}
			if p.Audio.ID != "" {
				resp.SubmittedAudio = &struct {
					ID string `json:"id"`
				}{ID: p.Audio.ID}
			}
		}
	}
	// IsPassed / IsCompleted 直接传递指针
	resp.IsPassed = s.IsPassed
	resp.IsCompleted = s.IsCompleted
	resp.Feedback = s.Feedback
	resp.ReferenceAnswer = s.ReferenceAnswer
	resp.Duration = s.Duration
	// Type 按 submit_type 设置
	if s.SubmitType == "get-answer" {
		resp.Type = api.ZhaoZhouQiaoSumbitRespTypeGetAnswer
	}
	// OfferReferenceAnswer：已揭示答案时给前端显示获取答案按钮
	if s.Revealed {
		t := true
		resp.OfferReferenceAnswer = &t
	}
	return resp
}

// ToCulturalStyleDragSortResp 转换为「文化采风-拖拽排序」提交响应 DTO（api.CSBpDragSortSubmitResult）。
// payload_json 存 answer 数组（CSBpDragSortEntry 列表）。
func (s *NodeSubmission) ToCulturalStyleDragSortResp() api.CSBpDragSortSubmitResult {
	resp := api.CSBpDragSortSubmitResult{
		ID:           s.SubmitIDStr(),
		IsProcessing: s.IsProcessing,
		Answer:       []api.CSBpDragSortEntry{},
	}
	if len(s.PayloadJSON) > 0 {
		var p struct {
			Answer []api.CSBpDragSortEntry `json:"answer"`
		}
		if json.Unmarshal(s.PayloadJSON, &p) == nil && len(p.Answer) > 0 {
			resp.Answer = p.Answer
		}
	}
	if s.IsPassed != nil {
		resp.IsPassed = *s.IsPassed
	}
	if s.IsCompleted != nil {
		resp.IsCompleted = *s.IsCompleted
	}
	resp.Feedback = s.Feedback
	resp.Duration = s.Duration
	// ReferenceAnswer 从 reference_answer 字段解析
	if s.ReferenceAnswer != nil && *s.ReferenceAnswer != "" {
		var refs []api.CSBpDragSortEntry
		if json.Unmarshal([]byte(*s.ReferenceAnswer), &refs) == nil && len(refs) > 0 {
			resp.ReferenceAnswer = &refs
		}
	}
	return resp
}

// ToCulturalStyleQuickSelectResp 转换为「文化采风-快速选择」提交响应 DTO。
// 返回 submitId、完成状态和选中的选项 ID。
func (s *NodeSubmission) ToCulturalStyleQuickSelectResp() CulturalStyleQuickSelectSubmitResult {
	resp := CulturalStyleQuickSelectSubmitResult{
		ID:           s.SubmitIDStr(),
		IsProcessing: s.IsProcessing,
		IsCompleted:  true,
		IsPassed:     true,
	}
	if s.IsCompleted != nil {
		resp.IsCompleted = *s.IsCompleted
	}
	// 从 payloadJSON 中提取 selectedId
	if len(s.PayloadJSON) > 0 {
		var p struct {
			SelectedID string `json:"selectedId"`
		}
		if json.Unmarshal(s.PayloadJSON, &p) == nil && p.SelectedID != "" {
			resp.SelectedID = &p.SelectedID
		}
	}
	return resp
}

// CulturalStyleQuickSelectSubmitResult 快速选择提交结果。
type CulturalStyleQuickSelectSubmitResult struct {
	ID           string  `json:"id"`
	IsProcessing bool    `json:"isProcessing"`
	IsPassed     bool    `json:"isPassed"`
	IsCompleted  bool    `json:"isCompleted"`
	SelectedID   *string `json:"selectedId,omitempty"`
}

// ToWenMingZhongWaiResp 转换为「文明中外」提交响应 DTO（api.WenMingZhongWaiSubmitResult）。
// payload_json 存 answers map[rowId]answer。
func (s *NodeSubmission) ToWenMingZhongWaiResp() api.WenMingZhongWaiSubmitResult {
	resp := api.WenMingZhongWaiSubmitResult{
		ID:           s.SubmitIDStr(),
		IsProcessing: s.IsProcessing,
		Answers:      map[string]string{},
	}
	if len(s.PayloadJSON) > 0 {
		var p struct {
			Answers map[string]string `json:"answers"`
		}
		if json.Unmarshal(s.PayloadJSON, &p) == nil && p.Answers != nil {
			resp.Answers = p.Answers
		}
	}
	resp.IsCompleted = s.IsCompleted
	resp.Feedback = s.Feedback
	resp.Duration = s.Duration
	// ReferenceAnswer：从 result_json 或 reference_answer 提取
	if s.ReferenceAnswer != nil {
		// 单字符串形式的参考答案，尝试作为整体 JSON 解析；不行则忽略
		var m map[string]string
		if json.Unmarshal([]byte(*s.ReferenceAnswer), &m) == nil {
			resp.ReferenceAnswer = &m
		}
	}
	if len(s.ResultJSON) > 0 {
		var r struct {
			Errors []struct {
				RowID   string  `json:"rowId"`
				BlankID string  `json:"blankId"`
				Msg     *string `json:"msg"`
			} `json:"errors"`
			ReferenceAnswer map[string]string `json:"referenceAnswer"`
			Ending          *struct {
				Instruction string `json:"instruction"`
				Video       struct {
					ID  string `json:"id"`
					Src string `json:"src"`
				} `json:"video"`
			} `json:"ending"`
		}
		if json.Unmarshal(s.ResultJSON, &r) == nil {
			if len(r.Errors) > 0 {
				errs := make([]struct {
					BlankID string  `json:"blankId"`
					Msg     *string `json:"msg,omitempty"`
					RowID   string  `json:"rowId"`
				}, len(r.Errors))
				for i, e := range r.Errors {
					errs[i] = struct {
						BlankID string  `json:"blankId"`
						Msg     *string `json:"msg,omitempty"`
						RowID   string  `json:"rowId"`
					}{BlankID: e.BlankID, Msg: e.Msg, RowID: e.RowID}
				}
				resp.Errors = &errs
			}
			if r.ReferenceAnswer != nil {
				resp.ReferenceAnswer = &r.ReferenceAnswer
			}
			if r.Ending != nil {
				resp.Ending = &struct {
					Instruction string `json:"instruction"`
					Video       struct {
						ID  string `json:"id"`
						Src string `json:"src"`
					} `json:"video"`
				}{
					Instruction: r.Ending.Instruction,
					Video: struct {
						ID  string `json:"id"`
						Src string `json:"src"`
					}{ID: r.Ending.Video.ID, Src: r.Ending.Video.Src},
				}
			}
		}
	}
	return resp
}

// ToHeritageCulturalResp 转换为「讲解优秀文化」提交响应 DTO（api.HeritageCulturalSubmitResult）。
// payload_json 存 {"text": "..."}。
func (s *NodeSubmission) ToHeritageCulturalResp() api.HeritageCulturalSubmitResult {
	resp := api.HeritageCulturalSubmitResult{
		ID:           s.SubmitIDStr(),
		IsProcessing: s.IsProcessing,
	}
	resp.IsPassed = s.IsPassed
	resp.IsCompleted = s.IsCompleted
	resp.Feedback = s.Feedback
	resp.ReferenceAnswer = s.ReferenceAnswer
	// SubmittedText 从 payload_json 提取
	if len(s.PayloadJSON) > 0 {
		var p struct {
			Text string `json:"text"`
		}
		if json.Unmarshal(s.PayloadJSON, &p) == nil {
			resp.SubmittedText = p.Text
		}
	}
	// Duration 直接使用模型字段
	resp.Duration = s.Duration
	return resp
}
