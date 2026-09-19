package service

// --- 讲解优秀文化（heritage-cultural）AI 输入输出结构 ---

// HeritageJudgeAIInput 判定提示词（heritage-judge.md）输入数据包。
// 对齐 prompts/heritage-judge.md：student_input 为合格判定的唯一分析对象。
type HeritageJudgeAIInput struct {
	StudentInput string `json:"student_input"`
}

// HeritageJudgeAIResult 判定提示词输出（严格 JSON，仅含 is_qualified）。
type HeritageJudgeAIResult struct {
	IsQualified bool `json:"is_qualified"`
}

// HeritageCoachingHistoryItem 辅导提示词（heritage-coaching.md）中的单条历史交互记录。
type HeritageCoachingHistoryItem struct {
	SubmissionIndex int    `json:"submission_index"`
	StudentInput    string `json:"student_input"`
	AIFeedback      string `json:"ai_feedback"`
	IsQualified     bool   `json:"is_qualified"`
}

// HeritageCoachingAIInput 辅导提示词（heritage-coaching.md）输入数据包。
// 字段对齐 prompts/heritage-coaching.md 的 student_data 与 system_control。
type HeritageCoachingAIInput struct {
	StudentName     string                        `json:"student_name"`
	Gender          string                        `json:"gender"`
	City            string                        `json:"city"`
	IsQualified     bool                          `json:"is_qualified"`
	SubmissionIndex int                           `json:"submission_index"`
	MaxSubmissions  int                           `json:"max_submissions"`
	StudentInput    string                        `json:"student_input"`
	History         []HeritageCoachingHistoryItem `json:"history"`
	ResponseMode    string                        `json:"response_mode"` // "仅辅导" | "辅导+范文"
}

// HeritageCoachingAIResult 辅导提示词输出（严格 JSON，仅含 feedback/revised_example）。
type HeritageCoachingAIResult struct {
	Feedback       string `json:"feedback"`
	RevisedExample string `json:"revised_example"`
}

// HeritageCulturalEndingResp 结束任务返回的信息（OpenAPI 中为 inline 对象）。
//
// 双分支：
//   - 节点已完成：返回综合评价（heritage-comprehensive 输出，含学伴反馈/教师评价/评分/关键词等）；
//   - 节点未完成：返回最后一次提交的辅导反馈（comment 字段），不触发综合评价。
type HeritageCulturalEndingResp struct {
	IsProcessing  bool   `json:"isProcessing"`
	Passed        bool   `json:"passed"`
	Comment       string `json:"comment"`
	ErrorCount    int    `json:"errorCount"`
	Revealed      bool   `json:"revealed"`
	CorrectAnswer string `json:"correctAnswer"`
	// 以下为节点完成后的综合评价字段（可选）
	StudentFeedback   string   `json:"studentFeedback,omitempty"`
	TeacherEvaluation string   `json:"teacherEvaluation,omitempty"`
	TeacherSuggestion string   `json:"teacherSuggestion,omitempty"`
	ScoreGrade        string   `json:"scoreGrade,omitempty"`
	ScoreValue        string   `json:"scoreValue,omitempty"`
	ScoreReason       string   `json:"scoreReason,omitempty"`
	CultureKeywords   []string `json:"cultureKeywords,omitempty"`
	ErrorKeywords     []string `json:"errorKeywords,omitempty"`
}

// heritageCulturalResultJSON 讲解优秀文化提交回写的结构化结果（写入 node_submissions.result_json）。
// 用于持久化每次提交的判定结果与优化范文，供前端 DTO 与综合评价复用。
type heritageCulturalResultJSON struct {
	IsQualified    bool   `json:"is_qualified"`
	RevisedExample string `json:"revised_example"`
}
