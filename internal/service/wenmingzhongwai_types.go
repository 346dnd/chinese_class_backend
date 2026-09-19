package service

// WenMingZhongWaiSubmitRequest 文明中外提交请求（来自 SubmitWenMingZhongWaiJSONBody）。
type WenMingZhongWaiSubmitRequest struct {
	Answers map[string]string // key = rowId（即 blankId）
	Type    string            // "submit" | "get-answer"
}

// --- AI 辅导输入（prompts/wenmingzhongwai.md 对齐） ---

// wenMingZhongWaiTaskSlot 单个填空位的定义（task_structure.slots 元素）。
type wenMingZhongWaiTaskSlot struct {
	SlotID      string `json:"slot_id"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

// wenMingZhongWaiTaskStructure 任务表格结构定义。
type wenMingZhongWaiTaskStructure struct {
	Slots []wenMingZhongWaiTaskSlot `json:"slots"`
}

// wenMingZhongWaiAIHistoryItem 历史对话记录（之前的作答与 AI 反馈）。
type wenMingZhongWaiAIHistoryItem struct {
	StudentInput map[string]string `json:"student_input"`
	AIFeedback   string            `json:"ai_feedback"`
}

// wenMingZhongWaiAIInput 传给 AI 生成辅导反馈的输入数据包。
type wenMingZhongWaiAIInput struct {
	StudentName     string                         `json:"student_name"`
	Gender          string                         `json:"gender"`
	City            string                         `json:"city"`
	TaskStructure   wenMingZhongWaiTaskStructure   `json:"task_structure"`
	ErrorSlot       string                         `json:"error_slot"`
	StudentInput    map[string]string              `json:"student_input"`
	ReferenceAnswer map[string]string              `json:"reference_answer"`
	Context         string                         `json:"context"`
	Strategy        string                         `json:"strategy"`
	History         []wenMingZhongWaiAIHistoryItem `json:"history"`
}

// wenMingZhongWaiAIResult AI 返回的辅导反馈结果。
type wenMingZhongWaiAIResult struct {
	FeedbackText string `json:"feedback_text"`
}

// --- 提交结果结构化回写（result_json） ---

// wenMingZhongWaiErrorItem 出错行的提示信息。
type wenMingZhongWaiErrorItem struct {
	RowID   string  `json:"rowId"`
	BlankID string  `json:"blankId"`
	Msg     *string `json:"msg,omitempty"`
}

// wenMingZhongWaiEndingJSON 完成时返回的总结信息。
type wenMingZhongWaiEndingJSON struct {
	Instruction string `json:"instruction"`
	Video       struct {
		ID  string `json:"id"`
		Src string `json:"src"`
	} `json:"video"`
}

// wenMingZhongWaiResultJSON result_json 结构化回写内容。
// Errors/ReferenceAnswer/Ending 供前端 DTO 还原（见 ToWenMingZhongWaiResp）；
// RowStates 为内部行级状态跟踪字段（passed/failed/completed），DTO 解析时忽略。
type wenMingZhongWaiResultJSON struct {
	Errors          []wenMingZhongWaiErrorItem `json:"errors"`
	ReferenceAnswer map[string]string          `json:"referenceAnswer"`
	Ending          *wenMingZhongWaiEndingJSON `json:"ending"`
	RowStates       map[string]string          `json:"rowStates,omitempty"`
}
