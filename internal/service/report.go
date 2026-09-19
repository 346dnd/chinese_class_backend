package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"zhonghuawenhua_backend/internal/api"
	"zhonghuawenhua_backend/internal/config"
	"zhonghuawenhua_backend/internal/model"
	"zhonghuawenhua_backend/pkg/ai"
)

// reportEvaluationDTO 与 api.NodeEvaluation.Evaluation（内联匿名结构体）保持一致。
// 类型别名确保可直接赋值给 api.NodeEvaluation.Evaluation。
type reportEvaluationDTO = struct {
	Comment *string `json:"comment,omitempty"`
	Items   *[]struct {
		Text  string `json:"text"`
		Title string `json:"title"`
	} `json:"items,omitempty"`
	Rating      *api.NodeEvaluationEvaluationRating `json:"rating,omitempty"`
	RatingColor *struct {
		Bg string `json:"bg"`
		Fg string `json:"fg"`
	} `json:"ratingColor,omitempty"`
	Title string `json:"title"`
}

// ReportService 课堂报告业务。
//
// 提供两个接口：
//   - GetStuNodeReport：单节点报告（AI 综合评价 + 每题/总用时 + 错误回答 AI 讲解 + 积分）
//   - GetStuClassReport：班级汇总报告（节点状态 + 积分/用时/获赞等汇总指标）
//
// AI 综合评价统一使用各节点的 comprehensive 提示词（prompts 目录内 *.comprehensive.md，
// 通过 prompts.Types 按 key 读取）。节点尚未提供 comprehensive 提示词时（如
// cultural-style-comprehensive / zhaozhouqiao-comprehensive 等待补充），降级返回
// 基于数据的简要评价，待提示词补齐后自动接入。
type ReportService struct {
	repos   *Repositories
	llm     ai.LLMClient
	prompts config.PromptsConfig
	svcs    *Services // 复用各节点 GetState 组装学习过程数据
}

// NewReportService 创建 ReportService。svcs 用于复用各子服务的状态组装能力。
func NewReportService(repos *Repositories, llm ai.LLMClient, prompts config.PromptsConfig, svcs *Services) *ReportService {
	return &ReportService{repos: repos, llm: llm, prompts: prompts, svcs: svcs}
}

// GetStuNodeReport 获取单节点报告。
// 返回：学习过程数据（Details.Process.Data，含每题的提交记录与错误回答 AI 讲解）、
// 节点获得积分（每题答对 = 1 分）、完成总用时（秒）。
// 注：AI 综合评价（api.NodeEvaluation）由独立接口 GET /evaluation 返回，见 GetNodeEvaluation。
func (s *ReportService) GetStuNodeReport(ctx context.Context, classID, nodeID, userID int64) (*api.NodeReport, error) {
	if err := s.repos.EnsureClassMember(ctx, classID, userID); err != nil {
		return nil, err
	}
	node, err := s.repos.ClassNode.GetByID(ctx, nodeID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNodeNotFound
		}
		return nil, err
	}
	if node.ClassID != classID {
		return nil, ErrNodeNotInClass
	}

	stats := buildNodeStats(ctx, s.repos, node.NodeType, node.ID, userID)
	process := s.buildNodeProcessData(ctx, classID, node.ID, userID, node.NodeType)

	details := &api.ReportDetails{}
	details.Process.Title = "你的学习过程"
	details.Process.Description = fmt.Sprintf("此处记录了你在“%s”任务中全部学习轨迹", node.Title)
	details.Process.Data = process

	points := stats.Points
	totalTime := stats.TotalTime
	return &api.NodeReport{
		ID:           int(node.ID),
		Title:        node.Title,
		Details:      details,
		PointsEarned: &points,
		Time:         &totalTime,
	}, nil
}

// GetNodeEvaluation 获取节点综合评价（独立接口 GET /evaluation，返回 api.NodeEvaluation）。
// 分支语义与 buildReportEvaluation 对齐：
//   - 有缓存/已生成：返回完整评价（Evaluation 有值，IsProcessing=false）；
//   - 生成中（同步生成超时/失败）：返回 IsProcessing=true，前端显示"AI 生成中，稍后刷新"；
//   - 节点未完成：返回 ErrEvaluationNotReady（前端提示继续作答）；
//   - 无法生成（未配置 LLM / 无 comprehensive 提示词 / 无作答记录）：降级返回基础评价。
func (s *ReportService) GetNodeEvaluation(ctx context.Context, classID, nodeID, userID int64) (*api.NodeEvaluation, error) {
	if err := s.repos.EnsureClassMember(ctx, classID, userID); err != nil {
		return nil, err
	}
	node, err := s.repos.ClassNode.GetByID(ctx, nodeID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNodeNotFound
		}
		return nil, err
	}
	if node.ClassID != classID {
		return nil, ErrNodeNotInClass
	}

	stats := buildNodeStats(ctx, s.repos, node.NodeType, node.ID, userID)
	if stats.Input != nil {
		stats.Input.NodeTitle = node.Title
	}
	evaluation, err := s.buildReportEvaluation(ctx, node.Title, node.NodeType, stats.Input)
	if err != nil {
		if errors.Is(err, ErrEvaluationGenerating) {
			return &api.NodeEvaluation{IsProcessing: true}, nil
		}
		return nil, err
	}
	return &api.NodeEvaluation{Evaluation: &evaluation}, nil
}

// GetStuClassReport 获取班级汇总报告。
// 返回课堂标题、各学习节点状态，以及汇总指标：总积分、最近一次答对积分（PointsChange）、
// 已完成节点学习用时总和、完成节点数、获赞数、获得评论数等。
func (s *ReportService) GetStuClassReport(ctx context.Context, classID, userID int64) (*api.GetStuClassReport, error) {
	if err := s.repos.EnsureClassMember(ctx, classID, userID); err != nil {
		return nil, err
	}
	cls, err := s.repos.Class.GetByID(ctx, classID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNodeNotFound
		}
		return nil, err
	}
	nodes, err := s.repos.ClassNode.ListByClassID(ctx, classID)
	if err != nil {
		return nil, err
	}

	// 仅统计学习节点（排除根节点/章节容器/文件夹）
	learnNodes := make([]model.ClassNode, 0, len(nodes))
	for _, n := range nodes {
		if n.ParentID == nil {
			continue
		}
		if _, ok := nodeTypeToKey(n.NodeType); !ok {
			continue
		}
		learnNodes = append(learnNodes, n)
	}

	reportNodes := make([]struct {
		ID     string                       `json:"id"`
		Status api.BaseActivityStatusString `json:"status"`
		Title  string                       `json:"title"`
	}, 0, len(learnNodes))

	totalPoints, studyDuration, finishedCount := 0, 0, 0
	lastCorrectPoints := 0
	for _, n := range learnNodes {
		stats := buildNodeStats(ctx, s.repos, n.NodeType, n.ID, userID)

		// 节点状态：未开始 / 进行中 / 已完成（仅当节点内所有题目均完成才算完成）
		status := api.NotStarted
		if stats.HasStarted {
			if stats.Completed {
				status = api.Finished
			} else {
				status = api.InProgress
			}
		}
		reportNodes = append(reportNodes, struct {
			ID     string                       `json:"id"`
			Status api.BaseActivityStatusString `json:"status"`
			Title  string                       `json:"title"`
		}{ID: strconv.FormatInt(n.ID, 10), Status: status, Title: n.Title})

		// 积分 / 用时 / 最近答对积分
		totalPoints += stats.Points
		if stats.LastCorrectPoints > 0 {
			lastCorrectPoints = stats.LastCorrectPoints
		}
		if stats.Completed {
			finishedCount++
			if stats.TotalTime > 0 {
				studyDuration += stats.TotalTime
			}
		}
	}

	// 朋友圈汇总（获赞数 / 获得评论数）
	likeCount, commentCount := 0, 0
	if stats, err := s.svcs.Moment.GetStats(ctx, classID, userID); err == nil {
		likeCount = stats.LikesReceived
		commentCount = stats.CommentsReceived
	}

	summary := struct {
		CommentReceivedCount int `json:"commentReceivedCount"`
		FinishedNodeCount    int `json:"finishedNodeCount"`
		LikeCount            int `json:"likeCount"`
		NodeCount            int `json:"nodeCount"`
		Points               int `json:"points"`
		PointsChange         int `json:"pointsChange"`
		StudyDuration        int `json:"studyDuration"`
	}{
		CommentReceivedCount: commentCount,
		FinishedNodeCount:    finishedCount,
		LikeCount:            likeCount,
		NodeCount:            len(learnNodes),
		Points:               totalPoints,
		PointsChange:         lastCorrectPoints,
		StudyDuration:        studyDuration,
	}

	return &api.GetStuClassReport{
		Title:   cls.Name,
		Nodes:   reportNodes,
		Summary: summary,
	}, nil
}

// --- 节点统计数据 ---

// nodeReportStats 节点报告的统计数据。
type nodeReportStats struct {
	Input             *reportComprehensiveInput // AI 综合评价输入
	Points            int                       // 节点获得积分（每题答对 = 1 分）
	TotalTime         int                       // 节点完成总用时（秒）
	LastCorrectPoints int                       // 最近一次答对获得的积分（班级报告 PointsChange 用）
	HasStarted        bool                      // 是否已开始（存在任意提交记录）
	Completed         bool                      // 节点是否全部题目完成（每个题/空均有完成记录才算）
}

// buildNodeStats 计算节点报告的积分、总用时与最近答对积分，并组装 AI 综合评价输入。
// 输入覆盖每题/每空的全部提交记录（含正确与错误），每条记录携带学生答案、正确答案、
// 正误、错误时的 AI 讲解（Feedback）与耗时，供 comprehensive 提示词使用。
func buildNodeStats(ctx context.Context, repos *Repositories, nt model.NodeType, nodeID, userID int64) nodeReportStats {
	// 文明中外为整表提交（一次提交覆盖多行），分段/逐条耗时模型不适用，
	// 使用独立的矩阵统计 builder（含对齐 wenmingzhongwai-comprehensive.md 的 AI 输入）。
	if nt == model.NodeTypeWenMingZhongWai {
		return buildWenMingZhongWaiNodeStats(ctx, repos, nodeID, userID)
	}
	// 讲解优秀文化为单文本多次提交（无题目维度），使用独立的统计 builder
	// （含对齐 heritage-comprehensive.md 的 AI 输入，字段与通用 records 不同）。
	if nt == model.NodeTypeHeritageCultural {
		return buildHeritageCulturalNodeStats(ctx, repos, nodeID, userID)
	}

	subs, err := repos.NodeSubmission.ListByUserNode(ctx, userID, nodeID, 500, 0)
	if err != nil {
		return nodeReportStats{}
	}

	// 确定总用时的起始基准 entryTime：
	//   - 写感想/初步感悟等节点记录进入时间（draft.entered_at），优先采用；
	//   - 未记录进入时间的节点（赵州桥/文明中外/文化采风等）回退用最早一条提交时间；
	//   - 进入时间与首次提交跨天（如多日前进入节点、本次才作答）或晚于首次提交时视为不可靠，
	//     同样截断为最早提交时间，避免总用时被算成数天。
	entryTime := nodeEntryTime(ctx, repos, nt, nodeID, userID)
	var firstSub time.Time
	for _, s := range subs {
		if s.SubmittedAt.IsZero() {
			continue
		}
		if firstSub.IsZero() || s.SubmittedAt.Before(firstSub) {
			firstSub = s.SubmittedAt
		}
	}
	if !firstSub.IsZero() && (entryTime.IsZero() || !sameCalendarDay(entryTime, firstSub) || entryTime.After(firstSub)) {
		entryTime = firstSub
	}
	segments, keyOf, titleBy, correctBy := buildReportSegments(ctx, repos, nt, nodeID)
	entries, totalTime := ComputeSegmentDurations(subs, entryTime, segments, keyOf, isKeyCompletion)

	subCount := make(map[string]int)
	passedBySeg := make(map[string]bool)
	completedBySeg := make(map[string]bool)
	records := make([]reportRecord, 0, len(entries))
	for _, e := range entries {
		sub := e.Sub
		seg := keyOf(sub)
		if seg == "" {
			continue
		}
		var p map[string]string
		if len(sub.PayloadJSON) > 0 {
			_ = json.Unmarshal(sub.PayloadJSON, &p)
		}
		subCount[seg]++
		isPass := sub.IsPassed != nil && *sub.IsPassed
		if isPass {
			passedBySeg[seg] = true
		}
		rec := reportRecord{
			SegmentTitle:  titleBy[seg],
			AttemptIndex:  subCount[seg],
			StudentInput:  extractReportInput(p),
			IsPass:        isPass,
			TimeCost:      e.Duration,
			UsedReference: sub.SubmitType == "get-answer",
		}
		if correctBy[seg] != "" {
			rec.CorrectAnswer = correctBy[seg]
		}
		if sub.Feedback != nil {
			rec.Feedback = *sub.Feedback // 错误回答的 AI 讲解
		}
		records = append(records, rec)
	}

	// 节点完成判定：遍历全部提交（含 get-answer 揭示），所有题目/空位均有完成记录才算完成。
	// 注意不可用 entries（仅含 submit 记录）判定，否则 get-answer 完成的题目会被漏计。
	for _, sub := range subs {
		seg := keyOf(sub)
		if seg == "" {
			continue
		}
		if isKeyCompletion(sub) {
			completedBySeg[seg] = true
		}
	}

	// 积分：每道题最终答对得 1 分（同一题多次提交只计一次）
	points := 0
	for _, passed := range passedBySeg {
		if passed {
			points++
		}
	}

	// PointsChange：最近一次答对获得的积分（每题答对 = 1 分，存在任意答对即 = 1）
	lastCorrect := 0
	if len(passedBySeg) > 0 {
		lastCorrect = 1
	}

	// 节点完成：所有题目/空位均有完成记录（答对或达到上限揭示）才算完成
	completed := len(segments) > 0
	for _, k := range segments {
		if !completedBySeg[k] {
			completed = false
			break
		}
	}

	var name, gender, city string
	if u, err := repos.User.GetByID(ctx, userID); err == nil && u != nil {
		if u.RealName != nil {
			name = *u.RealName
		}
		if u.Gender != nil {
			gender = *u.Gender
		}
		if u.City != nil {
			city = *u.City
		}
	}

	input := &reportComprehensiveInput{
		StudentName:     name,
		Gender:          gender,
		City:            city,
		NodeType:        string(nt),
		Records:         records,
		TotalTime:       totalTime,
		TotalAttempts:   len(records),
		CorrectSegments: points,
		TotalSegments:   len(segments),
		PointsEarned:    points,
		NodeID:          nodeID,
		UserID:          userID,
		Completed:       completed,
	}
	return nodeReportStats{
		Input:             input,
		Points:            points,
		TotalTime:         totalTime,
		LastCorrectPoints: lastCorrect,
		HasStarted:        len(subs) > 0,
		Completed:         completed,
	}
}

// --- 文明中外矩阵统计（独立 builder） ---

// wenMingZhongWaiSlotDefinition 文明中外综合评价：全局任务定义中的填空位。
type wenMingZhongWaiSlotDefinition struct {
	SlotID        string `json:"slot_id"`
	Instruction   string `json:"instruction"`
	CorrectAnswer string `json:"correct_answer"`
}

// wenMingZhongWaiComprehensiveRecord 文明中外综合评价：单次提交行为记录。
type wenMingZhongWaiComprehensiveRecord struct {
	SubmissionIndex int               `json:"submission_index"`
	SubmittedSlots  []string          `json:"submitted_slots"`
	StudentAnswers  map[string]string `json:"student_answers"`
	IsAllCorrect    bool              `json:"is_all_correct"`
	WrongSlots      []string          `json:"wrong_slots"`
	TimeCost        int               `json:"time_cost"`
}

// wenMingZhongWaiComprehensiveData 文明中外综合评价 AI 输入数据包（对齐 wenmingzhongwai-comprehensive.md）。
type wenMingZhongWaiComprehensiveData struct {
	TaskContext struct {
		SlotDefinitions []wenMingZhongWaiSlotDefinition `json:"slot_definitions"`
	} `json:"task_context"`
	StudentData struct {
		StudentName      string                               `json:"student_name"`
		Gender           string                               `json:"gender"`
		City             string                               `json:"city"`
		Records          []wenMingZhongWaiComprehensiveRecord `json:"records"`
		TotalTime        int                                  `json:"total_time"`
		TotalSubmissions int                                  `json:"total_submissions"`
		AccuracyRate     float64                              `json:"accuracy_rate"`
	} `json:"student_data"`
}

// buildWenMingZhongWaiNodeStats 文明中外（矩阵填空）节点报告统计。
//
// 与逐题节点的区别：一次 submit 覆盖多行，因此：
//   - 行级正误通过重判各次提交的 answers 计算（无需依赖 result_json）；
//   - 积分 = 最终答对的行数（每行 1 分）；总用时 = 末次提交时间 − 首次提交时间（无进入记录）；
//   - 完成 = 所有 input 行均已解决（答对 / 达上限自动填入 / get-answer 揭示）；
//   - AI 综合评价输入使用对齐 wenmingzhongwai-comprehensive.md 的独立数据包
//     （task_context.slot_definitions + student_data.records）。
func buildWenMingZhongWaiNodeStats(ctx context.Context, repos *Repositories, nodeID, userID int64) nodeReportStats {
	params, err := loadWenMingZhongWaiParams(ctx, repos, nodeID)
	if err != nil {
		return nodeReportStats{}
	}
	subs, err := repos.NodeSubmission.ListByUserNode(ctx, userID, nodeID, 500, 0)
	if err != nil {
		return nodeReportStats{}
	}

	rows := params.inputRowIndices()
	type rowStat struct {
		passed    bool
		completed bool
		errors    int
	}
	states := make(map[string]*rowStat, len(rows))
	for _, idx := range rows {
		states[params.Matrix.Rows[idx].RowID] = &rowStat{}
	}

	// 提交记录按时间升序（ListByUserNode 默认倒序）
	sorted := make([]model.NodeSubmission, 0, len(subs))
	for _, s := range subs {
		sorted = append(sorted, s)
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].SubmittedAt.Before(sorted[j].SubmittedAt) })

	// 单次提交行为记录（供 AI 综合评价）
	records := make([]wenMingZhongWaiComprehensiveRecord, 0, len(sorted))
	subIndex := 0
	submissionCount := 0
	prevTime := time.Time{}
	totalTime := 0
	var first, last time.Time

	for _, sub := range sorted {
		if sub.SubmitType == "get-answer" {
			// 一次性揭示答案：全部行视为完成
			for _, st := range states {
				st.completed = true
			}
			continue
		}
		if sub.SubmitType != "submit" {
			continue
		}
		submissionCount++
		subIndex++
		var p struct {
			Answers map[string]string `json:"answers"`
		}
		if len(sub.PayloadJSON) > 0 {
			_ = json.Unmarshal(sub.PayloadJSON, &p)
		}
		if p.Answers == nil {
			p.Answers = map[string]string{}
		}
		if first.IsZero() || sub.SubmittedAt.Before(first) {
			first = sub.SubmittedAt
		}
		if sub.SubmittedAt.After(last) {
			last = sub.SubmittedAt
		}
		// 本次提交耗时 = 本次提交时间 − 上一次提交时间（首条取 0）
		cost := 0
		if !prevTime.IsZero() && !sub.SubmittedAt.IsZero() {
			if d := int(sub.SubmittedAt.Sub(prevTime).Seconds()); d > 0 {
				cost = d
			}
		}
		prevTime = sub.SubmittedAt

		// 行级判定
		allCorrect := true
		var submittedSlots, wrongSlots []string
		for _, idx := range rows {
			rowID := params.Matrix.Rows[idx].RowID
			answer, ok := p.Answers[rowID]
			if !ok || strings.TrimSpace(answer) == "" {
				continue // 该行本次未提交
			}
			submittedSlots = append(submittedSlots, rowID)
			st := states[rowID]
			if params.gradeRow(rowID, answer) {
				st.passed = true
				st.completed = true
			} else {
				allCorrect = false
				wrongSlots = append(wrongSlots, rowID)
				st.errors++
			}
		}
		if len(submittedSlots) == 0 {
			continue
		}
		records = append(records, wenMingZhongWaiComprehensiveRecord{
			SubmissionIndex: subIndex,
			SubmittedSlots:  submittedSlots,
			StudentAnswers:  p.Answers,
			IsAllCorrect:    allCorrect,
			WrongSlots:      wrongSlots,
			TimeCost:        cost,
		})
	}

	// 达上限自动填入答案的行视为完成（与提交批改的 rowStates 规则一致）
	for _, st := range states {
		if !st.completed && st.errors >= params.MaxErrors {
			st.completed = true
		}
	}

	if !first.IsZero() && !last.IsZero() {
		if d := int(last.Sub(first).Seconds()); d > 0 {
			totalTime = d
		}
	}

	// 完成 / 积分 / 正确率
	completed := len(rows) > 0
	points, correctCount := 0, 0
	for _, st := range states {
		if st.passed {
			points++
			correctCount++
		}
		if !st.completed {
			completed = false
		}
	}
	lastCorrect := 0
	if points > 0 {
		lastCorrect = 1
	}
	accuracyRate := 0.0
	if len(rows) > 0 {
		accuracyRate = math.Round(float64(correctCount)/float64(len(rows))*100) / 100
	}

	// 学生信息
	var name, gender, city string
	if u, err := repos.User.GetByID(ctx, userID); err == nil && u != nil {
		if u.RealName != nil {
			name = *u.RealName
		}
		if u.Gender != nil {
			gender = *u.Gender
		}
		if u.City != nil {
			city = *u.City
		}
	}

	// 组装对齐 wenmingzhongwai-comprehensive.md 的 AI 输入数据包
	data := &wenMingZhongWaiComprehensiveData{}
	for _, idx := range rows {
		rowID := params.Matrix.Rows[idx].RowID
		data.TaskContext.SlotDefinitions = append(data.TaskContext.SlotDefinitions, wenMingZhongWaiSlotDefinition{
			SlotID:        rowID,
			Instruction:   params.rowLabel(rowID),
			CorrectAnswer: params.correctAnswer(rowID),
		})
	}
	data.StudentData.StudentName = name
	data.StudentData.Gender = gender
	data.StudentData.City = city
	data.StudentData.Records = records
	data.StudentData.TotalTime = totalTime
	data.StudentData.TotalSubmissions = submissionCount
	data.StudentData.AccuracyRate = accuracyRate

	input := &reportComprehensiveInput{
		StudentName:     name,
		Gender:          gender,
		City:            city,
		NodeType:        string(model.NodeTypeWenMingZhongWai),
		TotalTime:       totalTime,
		TotalAttempts:   submissionCount,
		CorrectSegments: points,
		TotalSegments:   len(rows),
		PointsEarned:    points,
		NodeID:          nodeID,
		UserID:          userID,
		Completed:       completed,
		WenMingZhongWai: data,
	}
	return nodeReportStats{
		Input:             input,
		Points:            points,
		TotalTime:         totalTime,
		LastCorrectPoints: lastCorrect,
		HasStarted:        len(subs) > 0,
		Completed:         completed,
	}
}

// --- 讲解优秀文化统计（独立 builder） ---

// heritageCulturalLearningRecord 讲解优秀文化综合评价：单条学习交互记录。
type heritageCulturalLearningRecord struct {
	SubmissionIndex int    `json:"submission_index"`
	StudentInput    string `json:"student_input"`
	IsQualified     bool   `json:"is_qualified"`
	SubmitTime      string `json:"submit_time"`
	AIFeedback      string `json:"ai_feedback"`
}

// heritageCulturalComprehensiveData 讲解优秀文化综合评价 AI 输入数据包（对齐 heritage-comprehensive.md）。
// 结构字段与该提示词约定的输入 JSON（student_profile + learning_records + max_submissions）一一对应。
type heritageCulturalComprehensiveData struct {
	StudentProfile struct {
		StudentName string `json:"student_name"`
		Gender      string `json:"gender"`
		City        string `json:"city"`
	} `json:"student_profile"`
	LearningRecords []heritageCulturalLearningRecord `json:"learning_records"`
	MaxSubmissions  int                              `json:"max_submissions"`
}

// buildHeritageCulturalNodeStats 讲解优秀文化（单文本多次提交）节点报告统计。
//
// 与逐题节点的区别：节点为单段文本、多次提交，无题目/空位维度，因此：
//   - 积分 = 存在合格提交即 1 分（PointsChange 同理）；
//   - 总用时 = 末次提交时间 − 首次提交时间（无进入记录）；
//   - 完成 = 存在合格提交或提交次数已达最大上限；
//   - AI 综合评价输入使用对齐 heritage-comprehensive.md 的独立数据包
//     （student_profile + learning_records + max_submissions）。
func buildHeritageCulturalNodeStats(ctx context.Context, repos *Repositories, nodeID, userID int64) nodeReportStats {
	params, err := loadHeritageCulturalParams(ctx, repos, nodeID)
	if err != nil {
		return nodeReportStats{}
	}
	subs, err := repos.NodeSubmission.ListByUserNode(ctx, userID, nodeID, 200, 0)
	if err != nil {
		return nodeReportStats{}
	}

	// 仅保留 submit 记录并按时间升序（ListByUserNode 默认倒序）
	sorted := make([]model.NodeSubmission, 0, len(subs))
	for _, s := range subs {
		if s.SubmitType == "submit" {
			sorted = append(sorted, s)
		}
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].SubmittedAt.Before(sorted[j].SubmittedAt) })

	// 学习交互记录（供 AI 综合评价）+ 合格/耗时聚合
	records := make([]heritageCulturalLearningRecord, 0, len(sorted))
	qualified := false
	totalTime := 0
	var first, last time.Time
	for i := range sorted {
		sub := &sorted[i]
		isPass := sub.IsPassed != nil && *sub.IsPassed
		if isPass {
			qualified = true
		}
		if first.IsZero() || sub.SubmittedAt.Before(first) {
			first = sub.SubmittedAt
		}
		if sub.SubmittedAt.After(last) {
			last = sub.SubmittedAt
		}
		var p struct {
			Text string `json:"text"`
		}
		if len(sub.PayloadJSON) > 0 {
			_ = json.Unmarshal(sub.PayloadJSON, &p)
		}
		fb := ""
		if sub.Feedback != nil {
			fb = *sub.Feedback
		}
		records = append(records, heritageCulturalLearningRecord{
			SubmissionIndex: i + 1,
			StudentInput:    p.Text,
			IsQualified:     isPass,
			SubmitTime:      sub.SubmittedAt.Format("2006-01-02 15:04:05"),
			AIFeedback:      fb,
		})
	}
	if !first.IsZero() && !last.IsZero() {
		if d := int(last.Sub(first).Seconds()); d > 0 {
			totalTime = d
		}
	}

	// 完成 / 积分
	submissionCount := len(sorted)
	completed := qualified || submissionCount >= params.MaxSubmissions
	points := 0
	lastCorrect := 0
	if qualified {
		points = 1
		lastCorrect = 1
	}

	// 学生信息
	var name, gender, city string
	if u, err := repos.User.GetByID(ctx, userID); err == nil && u != nil {
		if u.RealName != nil {
			name = *u.RealName
		}
		if u.Gender != nil {
			gender = *u.Gender
		}
		if u.City != nil {
			city = *u.City
		}
	}

	// 组装对齐 heritage-comprehensive.md 的 AI 输入数据包
	data := &heritageCulturalComprehensiveData{}
	data.StudentProfile.StudentName = name
	data.StudentProfile.Gender = gender
	data.StudentProfile.City = city
	data.LearningRecords = records
	data.MaxSubmissions = params.MaxSubmissions

	input := &reportComprehensiveInput{
		StudentName:      name,
		Gender:           gender,
		City:             city,
		NodeType:         string(model.NodeTypeHeritageCultural),
		TotalTime:        totalTime,
		TotalAttempts:    submissionCount,
		CorrectSegments:  points,
		TotalSegments:    1,
		PointsEarned:     points,
		NodeID:           nodeID,
		UserID:           userID,
		Completed:        completed,
		HeritageCultural: data,
	}
	return nodeReportStats{
		Input:             input,
		Points:            points,
		TotalTime:         totalTime,
		LastCorrectPoints: lastCorrect,
		HasStarted:        len(subs) > 0,
		Completed:         completed,
	}
}

// extractReportInput 从提交 payload 中提取学生作答文本。
func extractReportInput(p map[string]string) string {
	if v, ok := p["input"]; ok {
		return v
	}
	if v, ok := p["text"]; ok {
		return v
	}
	return ""
}

// nodeEntryTime 读取节点进入时间（仅已记录的节点类型有效）。
// 用于 ComputeSegmentDurations 计算首题/首空耗时基准。
func nodeEntryTime(ctx context.Context, repos *Repositories, nt model.NodeType, nodeID, userID int64) time.Time {
	switch nt {
	case model.NodeTypeWriteThoughts:
		var d writeThoughtsDraft
		_ = loadDraftPartition(ctx, repos, nodeID, userID, DraftKeyWriteThoughts, &d)
		if d.EnteredAt != nil {
			return *d.EnteredAt
		}
	case model.NodeTypeInitialInsight:
		var d initialInsightDraft
		_ = loadDraftPartition(ctx, repos, nodeID, userID, DraftKeyInitialInsight, &d)
		if d.EnteredAt != nil {
			return *d.EnteredAt
		}
	}
	return time.Time{}
}

// sameCalendarDay 判断两个时间是否在同一自然日。
func sameCalendarDay(a, b time.Time) bool {
	y1, m1, d1 := a.Date()
	y2, m2, d2 := b.Date()
	return y1 == y2 && m1 == m2 && d1 == d2
}

// --- AI 综合评价 ---

// reportRecord 传给 AI 的单条作答记录（含正确与错误）。
type reportRecord struct {
	SegmentTitle  string `json:"segment_title"`            // 题目/空位标题
	AttemptIndex  int    `json:"attempt_index"`            // 该题第几次作答
	StudentInput  string `json:"student_input"`            // 学生答案
	CorrectAnswer string `json:"correct_answer,omitempty"` // 正确答案（若有）
	IsPass        bool   `json:"is_pass"`                  // 是否答对
	Feedback      string `json:"feedback,omitempty"`       // 错误回答的 AI 讲解
	TimeCost      int    `json:"time_cost"`                // 本题作答耗时（秒）
	UsedReference bool   `json:"used_reference"`           // 是否使用参考答案
}

// reportComprehensiveInput 传给 AI 的节点综合评价输入数据包。
type reportComprehensiveInput struct {
	StudentName     string         `json:"student_name"`
	Gender          string         `json:"gender"`
	City            string         `json:"city"`
	NodeTitle       string         `json:"node_title"`
	NodeType        string         `json:"node_type"`
	Records         []reportRecord `json:"records"`
	TotalTime       int            `json:"total_time"`
	TotalAttempts   int            `json:"total_attempts"`
	CorrectSegments int            `json:"correct_segments"`
	TotalSegments   int            `json:"total_segments"`
	PointsEarned    int            `json:"points_earned"`

	// 以下为内部字段（json:"-" 不传给 AI），供报告读缓存与触发异步生成使用。
	NodeID    int64 `json:"-"`
	UserID    int64 `json:"-"`
	Completed bool  `json:"-"`

	// WenMingZhongWai 文明中外专用的综合评价数据包（对齐 wenmingzhongwai-comprehensive.md）。
	// 非空时，MarshalJSON 将只输出该数据包，忽略通用字段。
	WenMingZhongWai *wenMingZhongWaiComprehensiveData `json:"-"`
	// HeritageCultural 讲解优秀文化专用的综合评价数据包（对齐 heritage-comprehensive.md）。
	// 非空时，MarshalJSON 将只输出该数据包，忽略通用字段。
	HeritageCultural *heritageCulturalComprehensiveData `json:"-"`
}

// MarshalJSON 文明中外/讲解优秀文化节点时只输出其专用数据包，其他节点保持通用字段输出；别名避免递归。
func (in *reportComprehensiveInput) MarshalJSON() ([]byte, error) {
	if in.WenMingZhongWai != nil {
		return json.Marshal(in.WenMingZhongWai)
	}
	if in.HeritageCultural != nil {
		return json.Marshal(in.HeritageCultural)
	}
	type alias reportComprehensiveInput
	return json.Marshal((*alias)(in))
}

// flexibleString 兼容字符串与数字的 JSON 值（AI 返回的 score_value 可能为数字，
// 若按 string 解析会导致整个评价结果解析失败，因此做柔性转换）。
type flexibleString string

// UnmarshalJSON 兼容 string / number / bool / null 形式的 JSON 值。
func (f *flexibleString) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		var str string
		if err := json.Unmarshal(b, &str); err != nil {
			return err
		}
		*f = flexibleString(str)
		return nil
	}
	if s == "null" {
		*f = ""
		return nil
	}
	*f = flexibleString(s)
	return nil
}

// reportComprehensiveResult AI 返回的节点综合评价结果。
// 各节点 comprehensive 提示词输出的通用字段集合（字段按需填充）。
type reportComprehensiveResult struct {
	StudentFeedback       string         `json:"student_feedback"`
	TeacherEvaluation     string         `json:"teacher_evaluation"`
	TeacherSuggestion     string         `json:"teacher_suggestion"`
	ErrorTags             []string       `json:"error_tags"`
	ScoreGrade            string         `json:"score_grade"`
	ScoreValue            flexibleString `json:"score_value"`
	ScoreReason           string         `json:"score_reason"`
	CulturalPrideKeywords []string       `json:"cultural_pride_keywords"`
	// 讲解优秀文化（heritage-comprehensive）输出字段：culture_keywords（词云核心文化词）、
	// error_keywords（错误类型关键词）。与其他节点的 cultural_pride_keywords/error_tags 命名不同，
	// 单独字段承接，避免未知字段被静默丢弃。
	CultureKeywords []string `json:"culture_keywords"`
	ErrorKeywords   []string `json:"error_keywords"`
}

// reportComprehensivePromptKey 节点类型 → comprehensive 提示词 key。
// 未提供提示词的节点（如 cultural-style-comprehensive 等）返回空串，
// 待 prompts 目录补齐对应 *.comprehensive.md 后自动接入。
func reportComprehensivePromptKey(nt model.NodeType) string {
	switch nt {
	case model.NodeTypeWriteThoughts:
		return "write-comprehensive"
	case model.NodeTypeInitialInsight:
		return "initial-comprehensive"
	case model.NodeTypeCulturalStyle, model.NodeTypeCulturalStyleQuickSelect, model.NodeTypeCulturalStyleDragSort:
		return "cultural-style-comprehensive"
	case model.NodeTypeZhaoZhouQiao:
		return "zhaozhouqiao-comprehensive"
	case model.NodeTypeWenMingZhongWai:
		return "wenmingzhongwai-comprehensive"
	case model.NodeTypeHeritageCultural:
		return "heritage-comprehensive"
	case model.NodeTypeCreationWorkshop:
		return "creation-comprehensive"
	}
	return ""
}

// comprehensivePrompt 返回指定节点类型的 comprehensive 提示词内容。
func comprehensivePrompt(prompts config.PromptsConfig, nt model.NodeType) string {
	key := reportComprehensivePromptKey(nt)
	if key == "" {
		return ""
	}
	if p, ok := prompts.Types[key]; ok && p != "" {
		return p
	}
	return ""
}

// evalGenerationTimeout 综合评价同步生成/等待生成者完成的最长耗时。
// 提交完成时的异步预热通常已提前生成好，这里仅兜底竞态窗口，极少真正等待到上限。
const evalGenerationTimeout = 30 * time.Second

// nodeEvalFlight 节点综合评价生成的单飞锁。
// 同一 (nodeID, userID) 同一时间只允许一个 goroutine 实际调用 AI 生成，
// 其余请求（并发报告访问 / 提交预热）等待其完成后再读缓存，避免并发重复消耗 AI 调用。
type nodeEvalFlight struct {
	mu       sync.Mutex
	inflight map[string]chan struct{}
}

// evalFlightKey 生成单飞锁维度 key。
func evalFlightKey(nodeID, userID int64) string {
	return strconv.FormatInt(nodeID, 10) + ":" + strconv.FormatInt(userID, 10)
}

// start 尝试成为该 (nodeID, userID) 综合评价的生成者。
// 返回：
//   - isLeader=true：调用方负责执行生成，完成后必须调用 release 释放锁；
//   - isLeader=false：已有生成者在进行，等待 done 通知关闭后重新读缓存即可。
func (f *nodeEvalFlight) start(key string) (isLeader bool, done <-chan struct{}, release func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if ch, ok := f.inflight[key]; ok {
		return false, ch, nil
	}
	ch := make(chan struct{})
	f.inflight[key] = ch
	release = func() {
		f.mu.Lock()
		delete(f.inflight, key)
		close(ch)
		f.mu.Unlock()
	}
	return true, ch, release
}

// buildReportEvaluation 生成任务评价（ReportEvaluation）。
//
// 分支逻辑：
//   - 有缓存的 AI 综合评价：直接应用（秒回，不阻塞请求）；
//   - 节点未完成：返回 ErrEvaluationNotReady（前端提示"还没完成，继续作答"）；
//   - 节点已完成但无缓存：加锁同步生成（30s 超时），成功后返回完整评价；
//     生成超时或失败返回 ErrEvaluationGenerating（前端提示"AI 生成中，稍后刷新"）；
//   - 节点已完成但无法生成（未配置 LLM / 无 comprehensive 提示词 / 无作答记录）：
//     降级返回基于数据的简要评价。
func (s *ReportService) buildReportEvaluation(ctx context.Context, nodeTitle string, nt model.NodeType, input *reportComprehensiveInput) (reportEvaluationDTO, error) {
	if input == nil {
		input = &reportComprehensiveInput{}
	}
	ev := reportEvaluationDTO{
		Title: "任务评价",
		Comment: strPtrIfSet(fmt.Sprintf(
			"你完成了“%s”任务，共作答 %d 次，答对 %d 题，用时约 %d 秒。",
			nodeTitle, input.TotalAttempts, input.CorrectSegments, input.TotalTime)),
	}

	// 有缓存的 AI 综合评价则直接应用（不阻塞请求）
	if cached, err := loadCachedEvaluation(ctx, s.repos, input.NodeID, input.UserID); err == nil && cached != nil {
		s.applyComprehensiveResult(&ev, *cached)
		return ev, nil
	}

	// 节点未完成：综合评价尚不存在，提示继续作答
	if !input.Completed {
		return ev, ErrEvaluationNotReady
	}

	// 节点已完成但无法生成（未配置 LLM / 无 comprehensive 提示词 / 无作答记录）：降级返回基础评价
	if s.llm == nil || comprehensivePrompt(s.prompts, nt) == "" {
		return ev, nil
	}
	if len(input.Records) == 0 && input.WenMingZhongWai == nil && input.HeritageCultural == nil {
		return ev, nil
	}
	if input.WenMingZhongWai != nil && len(input.WenMingZhongWai.StudentData.Records) == 0 {
		return ev, nil
	}
	if input.HeritageCultural != nil && len(input.HeritageCultural.LearningRecords) == 0 {
		return ev, nil
	}

	// 节点已完成但缓存尚未生成：加锁同步生成（30s 超时）；失败视为"生成中"，让前端提示稍后刷新
	if err := ensureNodeEvaluation(ctx, s.repos, s.llm, s.prompts, nt, input.NodeID, input.UserID, nodeTitle); err != nil {
		return ev, err
	}
	if cached, err := loadCachedEvaluation(ctx, s.repos, input.NodeID, input.UserID); err == nil && cached != nil {
		s.applyComprehensiveResult(&ev, *cached)
	}
	return ev, nil
}

// ensureNodeEvaluation 确保节点综合评价已生成并落库。
// 已缓存则直接返回；无缓存时若自己成为生成者则同步调用 AI 生成（受 evalGenerationTimeout 约束），
// 否则等待现有生成者完成。生成失败/超时统一返回 ErrEvaluationGenerating。
// 报告路径与各节点 GetEnding 共用同一把单飞锁（repos.NodeEvalLock）。
func ensureNodeEvaluation(ctx context.Context, repos *Repositories, llm ai.LLMClient, prompts config.PromptsConfig, nt model.NodeType, nodeID, userID int64, nodeTitle string) error {
	key := evalFlightKey(nodeID, userID)
	isLeader, done, release := repos.NodeEvalLock.start(key)
	if isLeader {
		defer release()
		genCtx, cancel := context.WithTimeout(ctx, evalGenerationTimeout)
		defer cancel()
		if err := generateNodeEvaluation(genCtx, repos, llm, prompts, nt, nodeID, userID, nodeTitle); err != nil {
			log.Printf("node evaluation generate failed: %v", err)
			return ErrEvaluationGenerating
		}
		return nil
	}
	// 已有生成者在进行：等待其完成（同样受超时上限约束）
	select {
	case <-done:
		return nil
	case <-time.After(evalGenerationTimeout):
		return ErrEvaluationGenerating
	case <-ctx.Done():
		return ctx.Err()
	}
}

// loadCachedEvaluation 读取节点完成后异步缓存的 AI 综合评价结果。
// 无进度记录或尚未生成缓存时返回 (nil, nil)。
func loadCachedEvaluation(ctx context.Context, repos *Repositories, nodeID, userID int64) (*reportComprehensiveResult, error) {
	prog, err := repos.NodeProgress.GetByNodeUser(ctx, nodeID, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if len(prog.AIEvaluationJSON) == 0 || string(prog.AIEvaluationJSON) == "null" {
		return nil, nil
	}
	var res reportComprehensiveResult
	if err := json.Unmarshal(prog.AIEvaluationJSON, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// maybeTriggerNodeEvaluation 提交完成后检测节点是否全部完成；若完成且尚无缓存评价，则异步生成。
// 供 SubmitAsync 在 FinalizeProgress 之后调用，统一覆盖所有节点类型。
func maybeTriggerNodeEvaluation(ctx context.Context, repos *Repositories, llm ai.LLMClient, prompts config.PromptsConfig, nodeID, userID int64, nt model.NodeType) {
	if llm == nil || comprehensivePrompt(prompts, nt) == "" {
		return
	}
	stats := buildNodeStats(ctx, repos, nt, nodeID, userID)
	if !stats.Completed {
		return
	}
	// 已有缓存评价则不重复生成（避免并发/重复提交反复触发）
	prog, err := repos.NodeProgress.GetByNodeUser(ctx, nodeID, userID)
	if err == nil && len(prog.AIEvaluationJSON) > 0 && string(prog.AIEvaluationJSON) != "null" {
		return
	}
	nodeTitle := ""
	if n, err := repos.ClassNode.GetByID(ctx, nodeID); err == nil {
		nodeTitle = n.Title
	}
	triggerNodeEvaluationAsync(repos, llm, prompts, nt, nodeID, userID, nodeTitle)
}

// triggerNodeEvaluationAsync 节点完成后在独立后台 goroutine 中异步生成 AI 综合评价并缓存。
// 与报告路径同步生成共用同一把单飞锁：若已有生成在进行（如用户此刻正打开报告）则不重复调用 AI。
// 不占用用户提交锁、不阻塞提交流程，失败仅记录日志。
func triggerNodeEvaluationAsync(repos *Repositories, llm ai.LLMClient, prompts config.PromptsConfig, nt model.NodeType, nodeID, userID int64, nodeTitle string) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("async node evaluation panic: %v", r)
			}
		}()
		isLeader, _, release := repos.NodeEvalLock.start(evalFlightKey(nodeID, userID))
		if !isLeader {
			return
		}
		defer release()
		evalCtx, cancel := context.WithTimeout(context.Background(), evalGenerationTimeout)
		defer cancel()
		if err := generateNodeEvaluation(evalCtx, repos, llm, prompts, nt, nodeID, userID, nodeTitle); err != nil {
			log.Printf("async node evaluation failed: %v", err)
		}
	}()
}

// generateNodeEvaluation 生成节点 AI 综合评价并持久化到 node_progress.ai_evaluation_json。
// 仅当节点已完成且存在作答记录时执行。
func generateNodeEvaluation(ctx context.Context, repos *Repositories, llm ai.LLMClient, prompts config.PromptsConfig, nt model.NodeType, nodeID, userID int64, nodeTitle string) error {
	stats := buildNodeStats(ctx, repos, nt, nodeID, userID)
	if !stats.Completed || stats.Input == nil {
		return nil
	}
	// 文明中外/讲解优秀文化记录在专用数据包中，通用 Records 为空，需单独判断
	if len(stats.Input.Records) == 0 && stats.Input.WenMingZhongWai == nil && stats.Input.HeritageCultural == nil {
		return nil
	}
	if stats.Input.WenMingZhongWai != nil && len(stats.Input.WenMingZhongWai.StudentData.Records) == 0 {
		return nil
	}
	if stats.Input.HeritageCultural != nil && len(stats.Input.HeritageCultural.LearningRecords) == 0 {
		return nil
	}
	prompt := comprehensivePrompt(prompts, nt)
	if prompt == "" {
		return nil
	}
	stats.Input.NodeTitle = nodeTitle
	var res reportComprehensiveResult
	if err := callAIWithJSON(ctx, llm, prompt, stats.Input, &res); err != nil {
		return err
	}
	data, err := json.Marshal(res)
	if err != nil {
		return err
	}
	return repos.NodeProgress.SetAIEvaluation(ctx, nodeID, userID, data)
}

// applyComprehensiveResult 将 AI 综合评价结果映射到 ReportEvaluation。
func (s *ReportService) applyComprehensiveResult(ev *reportEvaluationDTO, res reportComprehensiveResult) {
	if res.StudentFeedback != "" {
		ev.Comment = strPtrIfSet(res.StudentFeedback)
	}
	items := make([]struct {
		Text  string `json:"text"`
		Title string `json:"title"`
	}, 0, 3)
	if res.TeacherEvaluation != "" {
		items = append(items, struct {
			Text  string `json:"text"`
			Title string `json:"title"`
		}{Title: "教师评价", Text: res.TeacherEvaluation})
	}
	if res.TeacherSuggestion != "" {
		items = append(items, struct {
			Text  string `json:"text"`
			Title string `json:"title"`
		}{Title: "成长建议", Text: res.TeacherSuggestion})
	}
	if res.ScoreReason != "" {
		items = append(items, struct {
			Text  string `json:"text"`
			Title string `json:"title"`
		}{Title: "评分说明", Text: res.ScoreReason})
	}
	if len(items) > 0 {
		ev.Items = &items
	}
	if res.ScoreGrade != "" {
		grade := api.NodeEvaluationEvaluationRating(strings.ToUpper(strings.TrimSpace(res.ScoreGrade)))
		ev.Rating = &grade
		color := ratingColorForGrade(string(grade))
		ev.RatingColor = &color
	}
}

// ratingColorForGrade 等级标签配色（A/B/C/D/E）。
func ratingColorForGrade(grade string) struct {
	Bg string `json:"bg"`
	Fg string `json:"fg"`
} {
	switch grade {
	case "A":
		return struct {
			Bg string `json:"bg"`
			Fg string `json:"fg"`
		}{Bg: "#52c41a", Fg: "#ffffff"}
	case "B":
		return struct {
			Bg string `json:"bg"`
			Fg string `json:"fg"`
		}{Bg: "#1890ff", Fg: "#ffffff"}
	case "C":
		return struct {
			Bg string `json:"bg"`
			Fg string `json:"fg"`
		}{Bg: "#faad14", Fg: "#ffffff"}
	case "D", "E":
		return struct {
			Bg string `json:"bg"`
			Fg string `json:"fg"`
		}{Bg: "#f5222d", Fg: "#ffffff"}
	}
	return struct {
		Bg string `json:"bg"`
		Fg string `json:"fg"`
	}{Bg: "#8c8c8c", Fg: "#ffffff"}
}

// --- 学习过程数据 ---

// buildNodeProcessData 组装学习过程数据（ReportProcessData 联合体）。
// 各节点复用对应子服务的 GetState（含每题的提交记录、正误、Feedback 错误讲解、耗时），
// 按节点类型映射到对应联合体变体。
func (s *ReportService) buildNodeProcessData(ctx context.Context, classID, nodeID, userID int64, nt model.NodeType) api.ReportProcessData {
	var data api.ReportProcessData
	switch nt {
	case model.NodeTypeWriteThoughts:
		if st, err := s.svcs.WriteThoughts.GetState(ctx, classID, nodeID, userID); err == nil {
			_ = data.FromReportProcessData0(api.ReportProcessData0{
				Type: string(nt),
				Data: struct {
					State api.WriteThoughtState `json:"state"`
				}{State: *st},
			})
		}
	case model.NodeTypeInitialInsight:
		if st, err := s.svcs.InitialInsight.GetState(ctx, classID, nodeID, userID); err == nil {
			_ = data.FromReportProcessData1(api.ReportProcessData1{
				Type: string(nt),
				Data: struct {
					State api.InitialInsightState `json:"state"`
				}{State: *st},
			})
		}
	case model.NodeTypeZhaoZhouQiao:
		if st, err := s.svcs.NodeState.GetZhaoZhouQiaoState(ctx, classID, nodeID, userID); err == nil {
			_ = data.FromReportProcessData3(api.ReportProcessData3{
				Type: string(nt),
				Data: struct {
					State api.ZhaoZhouQiaoState `json:"state"`
				}{State: *st},
			})
		}
	case model.NodeTypeWenMingZhongWai:
		if st, err := s.svcs.NodeState.GetWenMingZhongWaiState(ctx, classID, nodeID, userID); err == nil {
			var params *api.WenMingZhongWaiParams
			if p, err := s.svcs.NodeState.GetWenMingZhongWaiParams(ctx, nodeID); err == nil {
				params = p
			}
			_ = data.FromReportProcessData4(api.ReportProcessData4{
				Type: string(nt),
				Data: struct {
					Params api.WenMingZhongWaiParams `json:"params"`
					State  api.WenMingZhongWaiState  `json:"state"`
				}{State: *st},
			})
			if params != nil {
				_ = data.MergeReportProcessData4(api.ReportProcessData4{
					Type: string(nt),
					Data: struct {
						Params api.WenMingZhongWaiParams `json:"params"`
						State  api.WenMingZhongWaiState  `json:"state"`
					}{Params: *params, State: *st},
				})
			}
		}
	case model.NodeTypeHeritageCultural:
		title := ""
		if p, err := loadHeritageCulturalParams(ctx, s.repos, nodeID); err == nil {
			title = p.String
		}
		if st, err := s.svcs.HeritageCultural.GetState(ctx, classID, nodeID, userID); err == nil {
			_ = data.FromReportProcessData5(api.ReportProcessData5{
				Type: string(nt),
				Data: struct {
					State api.HeritageCulturalState `json:"state"`
					Title string                    `json:"title"`
				}{State: *st, Title: title},
			})
		}
	case model.NodeTypeCulturalStyle, model.NodeTypeCulturalStyleQuickSelect, model.NodeTypeCulturalStyleDragSort:
		// 文化采风：按交互点（拖拽排序 + 快速选择）组装学习过程数据
		records := s.buildCulturalStyleProcessData(ctx, classID, nodeID, userID)
		_ = data.FromReportProcessData2(api.ReportProcessData2{Type: string(nt), Data: records})
	case model.NodeTypeCreationWorkshop:
		// 创作工坊暂未接入报告数据
		_ = data.FromReportProcessData6(api.ReportProcessData6{Type: string(nt), Data: []struct {
			FnRecord api.ReportProcessData_6_Data_FnRecord `json:"fnRecord"`
			Title    string                                `json:"title"`
		}{}})
	}
	return data
}

// culturalStyleBpRecordDTO 与 api.ReportProcessData2.Data 元素类型（内联匿名结构体）保持一致。
type culturalStyleBpRecordDTO = struct {
	BpRecord api.ReportProcessData_2_Data_BpRecord `json:"bpRecord"`
	Title    string                                `json:"title"`
}

// buildCulturalStyleProcessData 组装文化采风学习过程数据（按交互点）。
//   - 拖拽排序：返回参数 + 状态（含该交互点全部提交记录），还原为 ReportProcessData2DataBpRecord1；
//   - 快速选择：从提交记录还原最后一次选择的选项文本，还原为 ReportProcessData2DataBpRecord0。
func (s *ReportService) buildCulturalStyleProcessData(ctx context.Context, classID, nodeID, userID int64) []culturalStyleBpRecordDTO {
	params, err := loadCulturalStyleParams(ctx, s.repos, nodeID)
	if err != nil || params.Breakpoints == nil {
		return nil
	}
	records := make([]culturalStyleBpRecordDTO, 0, len(params.Breakpoints.DragSort)+len(params.Breakpoints.QuickSelect))

	// 快速选择：从提交记录还原最后一次选择的选项 ID（ListByUserNode 按时间倒序，后出现的覆盖为最新）
	qsSelected := map[string]string{}
	if subs, err := s.repos.NodeSubmission.ListByUserNode(ctx, userID, nodeID, 500, 0); err == nil {
		for _, sub := range subs {
			if sub.QuestionID == nil || sub.SubmitType != "submit" {
				continue
			}
			if _, ok := params.Breakpoints.QuickSelect[*sub.QuestionID]; !ok {
				continue
			}
			var p struct {
				SelectedID string `json:"selectedId"`
			}
			if len(sub.PayloadJSON) > 0 {
				_ = json.Unmarshal(sub.PayloadJSON, &p)
			}
			if p.SelectedID != "" {
				qsSelected[*sub.QuestionID] = p.SelectedID
			}
		}
	}

	// 拖拽排序交互点
	for bpID, bp := range params.Breakpoints.DragSort {
		dsParams, err := s.svcs.CulturalStyle.GetDragSortParams(ctx, nodeID, bpID)
		if err != nil {
			continue
		}
		dsState, err := s.svcs.CulturalStyle.GetDragSortState(ctx, classID, nodeID, userID, bpID)
		if err != nil {
			continue
		}
		var br api.ReportProcessData_2_Data_BpRecord
		_ = br.FromReportProcessData2DataBpRecord1(api.ReportProcessData2DataBpRecord1{
			BpType: "drag-sort",
			Params: *dsParams,
			State:  *dsState,
		})
		records = append(records, culturalStyleBpRecordDTO{BpRecord: br, Title: bp.Title})
	}

	// 快速选择交互点
	for bpID, bp := range params.Breakpoints.QuickSelect {
		text := ""
		if selID := qsSelected[bpID]; selID != "" {
			for _, opt := range bp.Selections {
				if opt.ID == selID {
					text = opt.Text
					break
				}
			}
		}
		var br api.ReportProcessData_2_Data_BpRecord
		_ = br.FromReportProcessData2DataBpRecord0(api.ReportProcessData2DataBpRecord0{
			BpType: "quick-select",
			Selected: struct {
				Duration *int   `json:"duration,omitempty"`
				Text     string `json:"text"`
			}{Text: text},
		})
		records = append(records, culturalStyleBpRecordDTO{BpRecord: br, Title: bp.Title})
	}
	return records
}

// --- 分段信息 ---

// buildReportSegments 按节点类型构造报告分段信息（题目/空位顺序、标题、正确答案）。
// 返回分段键列表（segments）、记录→分段键映射函数（keyOf）、分段标题与正确答案表。
func buildReportSegments(ctx context.Context, repos *Repositories, nt model.NodeType, nodeID int64) (
	segments []string, keyOf func(sub model.NodeSubmission) string,
	titleBy map[string]string, correctBy map[string]string) {

	titleBy = map[string]string{}
	correctBy = map[string]string{}
	keyOf = func(sub model.NodeSubmission) string {
		if sub.QuestionID != nil {
			return *sub.QuestionID
		}
		return ""
	}

	switch nt {
	case model.NodeTypeWriteThoughts:
		params, err := loadWriteThoughtsParams(ctx, repos, nodeID)
		if err != nil {
			return nil, keyOf, titleBy, correctBy
		}
		for _, q := range params.Questions {
			segments = append(segments, q.ID)
			titleBy[q.ID] = q.Title
			if q.ReferenceAnswer != "" {
				correctBy[q.ID] = q.ReferenceAnswer
			}
		}
	case model.NodeTypeInitialInsight:
		params, err := loadInitialInsightParams(ctx, repos, nodeID)
		if err != nil {
			return nil, keyOf, titleBy, correctBy
		}
		idx := 0
		for _, q := range params.Questions {
			for _, c := range q.Content {
				if c.Type == "blank" && c.Blank != nil {
					idx++
					key := q.ID + ":" + c.Blank.ID
					segments = append(segments, key)
					titleBy[key] = fmt.Sprintf("%s·第%d空", q.Title, idx)
					if c.Blank.ReferenceAnswer != "" {
						correctBy[key] = c.Blank.ReferenceAnswer
					}
				}
			}
		}
		keyOf = func(sub model.NodeSubmission) string {
			if sub.QuestionID == nil {
				return ""
			}
			var p map[string]string
			if len(sub.PayloadJSON) > 0 {
				_ = json.Unmarshal(sub.PayloadJSON, &p)
			}
			return *sub.QuestionID + ":" + p["blankId"]
		}
	case model.NodeTypeZhaoZhouQiao:
		params, err := loadZhaoZhouQiaoParams(ctx, repos, nodeID)
		if err != nil {
			return nil, keyOf, titleBy, correctBy
		}
		for _, c := range params.Cards {
			segments = append(segments, c.ID)
			titleBy[c.ID] = c.Title
			if c.ReferenceAnswer != "" {
				correctBy[c.ID] = c.ReferenceAnswer
			}
		}
		keyOf = func(sub model.NodeSubmission) string {
			var p map[string]string
			if len(sub.PayloadJSON) > 0 {
				_ = json.Unmarshal(sub.PayloadJSON, &p)
			}
			if p["cardId"] != "" {
				return p["cardId"]
			}
			if sub.QuestionID != nil {
				return *sub.QuestionID
			}
			return ""
		}
	case model.NodeTypeCulturalStyle, model.NodeTypeCulturalStyleQuickSelect, model.NodeTypeCulturalStyleDragSort:
		params, err := loadCulturalStyleParams(ctx, repos, nodeID)
		if err != nil {
			return nil, keyOf, titleBy, correctBy
		}
		// 仅统计交互点（quick-select / drag-sort）——文化采风 questions 为展示型题目，
		// 无提交接口、永远不会有完成记录，若计入 segments 会导致节点永远无法判定为完成。
		if params.Breakpoints != nil {
			for id, bp := range params.Breakpoints.QuickSelect {
				segments = append(segments, id)
				titleBy[id] = bp.Title
				if bp.CorrectAnswer != "" {
					correctBy[id] = bp.CorrectAnswer
				}
			}
			for id, bp := range params.Breakpoints.DragSort {
				segments = append(segments, id)
				titleBy[id] = bp.Title
				if len(bp.CorrectOrder) > 0 {
					correctBy[id] = strings.Join(bp.CorrectOrder, " → ")
				}
			}
		}
	case model.NodeTypeWenMingZhongWai:
		segments = append(segments, "matrix")
		titleBy["matrix"] = "矩阵填空"
		params, err := loadWenMingZhongWaiParams(ctx, repos, nodeID)
		if err == nil {
			segments = segments[:0]
			for _, row := range params.Matrix.Rows {
				segments = append(segments, row.RowID)
				titleBy[row.RowID] = "第" + row.RowID + "行"
				if ans, ok := params.CorrectAnswers[row.RowID]; ok {
					correctBy[row.RowID] = ans
				}
			}
		}
	case model.NodeTypeHeritageCultural:
		segments = append(segments, "text")
		titleBy["text"] = "讲解优秀文化"
		params, err := loadHeritageCulturalParams(ctx, repos, nodeID)
		if err == nil && params.String != "" {
			titleBy["text"] = params.String
		}
		keyOf = func(sub model.NodeSubmission) string { return "text" }
	}
	return segments, keyOf, titleBy, correctBy
}
