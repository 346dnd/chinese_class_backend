package service

import (
	"context"
	"fmt"

	"zhonghuawenhua_backend/internal/api"
)

// --- 文明中外 params_json 结构 ---

// WenMingZhongWaiParams 文明中外节点的 params_json 结构。
// 直接复用 api.WenMingZhongWaiParams（含 Title/Description/IntroVideo/IntroBubbleText/Matrix）。
// 额外用 CorrectAnswers 保存每行 input 单元的正确答案，供批改使用。
type WenMingZhongWaiParams struct {
	api.WenMingZhongWaiParams
	// CorrectAnswers key = rowId，value = 正确答案。
	// 从 Matrix.Rows 中 type=correct-answer 的 cell.content 提取。
	CorrectAnswers map[string]string `json:"-"`
	// MaxErrors 允许的最大错误次数，到达后自动标记为已完成（默认 3）。
	MaxErrors int `json:"maxErrors,omitempty"`
	// Ending 完成时返回的总结信息（instruction + video）。
	Ending *WenMingZhongWaiEnding `json:"ending,omitempty"`
}

// WenMingZhongWaiEnding 完成时的总结信息。
type WenMingZhongWaiEnding struct {
	Instruction string `json:"instruction"`
	VideoID     string `json:"videoId"`
	VideoSrc    string `json:"videoSrc"`
}

// loadWenMingZhongWaiParams 加载文明中外节点参数。
func loadWenMingZhongWaiParams(ctx context.Context, repos *Repositories, nodeID int64) (*WenMingZhongWaiParams, error) {
	var p WenMingZhongWaiParams
	if err := loadNodeParams(ctx, repos, nodeID, &p); err != nil {
		return nil, err
	}
	if p.Title == "" || len(p.Matrix.Rows) == 0 {
		return nil, ErrNodeContentNotConfigured
	}
	// 提取正确答案：从矩阵每行的 type=correct-answer 单元格提取（seed.sql 已配置）
	p.CorrectAnswers = make(map[string]string)
	for _, row := range p.Matrix.Rows {
		for _, cell := range row.Cells {
			if cell.Type == api.MatrixRowsCellsType("correct-answer") && cell.Content != nil {
				p.CorrectAnswers[row.RowID] = *cell.Content
				break
			}
		}
	}
	if p.MaxErrors == 0 {
		p.MaxErrors = 3
	}
	return &p, nil
}

// wenMingZhongWaiParagraph 第3自然段原文，供 AI 辅导定位语境与寻找线索。
const wenMingZhongWaiParagraph = "画上的街市可热闹了。街上有挂着各种招牌的店铺、作坊、酒楼、茶馆……走在街上的，是来来往往、形态各异的人：有的骑着马，有的挑着担，有的赶着毛驴，有的推着独轮车，有的悠闲地在街上溜达。画面上的这些人，有的不到一寸，有的甚至只有黄豆那么大。别看画上的人小，每个人在干什么，都能看得清清楚楚。"

// inputRowIndices 返回矩阵中带 input 单元格的行下标（待作答的填空行），保持矩阵顺序。
func (p *WenMingZhongWaiParams) inputRowIndices() []int {
	var idxs []int
	for i, row := range p.Matrix.Rows {
		for _, cell := range row.Cells {
			if cell.Type == api.MatrixRowsCellsType("input") {
				idxs = append(idxs, i)
				break
			}
		}
	}
	return idxs
}

// isInputRow 判断指定 rowId 是否为带 input 单元格的作答行。
func (p *WenMingZhongWaiParams) isInputRow(rowID string) bool {
	for _, row := range p.Matrix.Rows {
		if row.RowID != rowID {
			continue
		}
		for _, cell := range row.Cells {
			if cell.Type == api.MatrixRowsCellsType("input") {
				return true
			}
		}
	}
	return false
}

// rowLabel 返回某行的左侧提示语（col1 单元格文本），供 AI 辅导精准定位。
func (p *WenMingZhongWaiParams) rowLabel(rowID string) string {
	for _, row := range p.Matrix.Rows {
		if row.RowID != rowID {
			continue
		}
		for _, cell := range row.Cells {
			if cell.ColumnKey == "col1" && cell.Content != nil {
				return *cell.Content
			}
		}
	}
	return ""
}

// correctAnswer 返回某行（填空）的标准答案；未知行返回空串。
func (p *WenMingZhongWaiParams) correctAnswer(rowID string) string {
	return p.CorrectAnswers[rowID]
}

// --- 文明中外 NodeStateService 方法 ---

// GetWenMingZhongWaiParams 获取文明中外节点静态参数。
// 返回页面固定信息（含 Matrix 表格定义）。
func (s *NodeStateService) GetWenMingZhongWaiParams(ctx context.Context, nodeID int64) (*api.WenMingZhongWaiParams, error) {
	params, err := loadWenMingZhongWaiParams(ctx, s.repos, nodeID)
	if err != nil {
		return nil, err
	}
	// 注意：返回给前端的 params 不含 CorrectAnswers / MaxErrors / Ending 等内部字段
	return &params.WenMingZhongWaiParams, nil
}

// GetWenMingZhongWaiState 获取文明中外节点状态。
// 返回该用户该节点的全部 submitLogs（按提交时间倒序）。
func (s *NodeStateService) GetWenMingZhongWaiState(ctx context.Context, classID, nodeID, userID int64) (*api.WenMingZhongWaiState, error) {
	if err := s.repos.EnsureClassMember(ctx, classID, userID); err != nil {
		return nil, err
	}
	// 加载 params 以取出 MaxErrors 和 Ending（用于提交响应的回写校验，此处 state 仅返回 submitLogs）
	if _, err := loadWenMingZhongWaiParams(ctx, s.repos, nodeID); err != nil {
		return nil, err
	}
	// 拉取该用户该节点全部提交记录
	submissions, err := s.repos.NodeSubmission.ListByUserNode(ctx, userID, nodeID, 200, 0)
	if err != nil {
		return nil, fmt.Errorf("list submissions: %w", err)
	}
	state := &api.WenMingZhongWaiState{SubmitLogs: []api.WenMingZhongWaiSubmitResult{}}
	for _, sub := range submissions {
		state.SubmitLogs = append(state.SubmitLogs, sub.ToWenMingZhongWaiResp())
	}
	return state, nil
}
