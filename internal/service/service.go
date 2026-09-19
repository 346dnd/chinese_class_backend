// Package service 封装业务逻辑，编排 repository 调用并产出 API 响应。
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"zhonghuawenhua_backend/internal/api"
	"zhonghuawenhua_backend/internal/config"
	"zhonghuawenhua_backend/internal/model"
	"zhonghuawenhua_backend/internal/repository"
	"zhonghuawenhua_backend/pkg/ai"
	"zhonghuawenhua_backend/pkg/ocr"
	"zhonghuawenhua_backend/pkg/stt"
	"zhonghuawenhua_backend/pkg/synclock"
	"zhonghuawenhua_backend/pkg/tts"
)

// Services 聚合所有业务 service，由 main 注入 handler。
type Services struct {
	WriteThoughts    *WriteThoughtsService
	InitialInsight   *InitialInsightService
	CulturalStyle    *CulturalStyleService
	AI               *AIService
	NodeState        *NodeStateService
	Account          *AccountService
	Moment           *MomentService
	HeritageCultural *HeritageCulturalService
	Upload           *UploadService
	ZhaoZhouQiao     *ZhaoZhouQiaoService
	WenMingZhongWai  *WenMingZhongWaiService
	Report           *ReportService
}

// NewServices 创建所有 service，注入 repository 依赖。
func NewServices(pool *pgxpool.Pool, llmClient ai.LLMClient, prompts config.PromptsConfig, jwtCfg config.JWTConfig, uploadDir, storageURL string, ocrCfg config.OCRConfig, sttCfg config.STTConfig, ttsCfg config.TTSConfig) *Services {
	base := repository.NewBase(pool)
	repos := &Repositories{
		SubmitLock:         synclock.NewUserSubmitLock(),
		MomentLock:         synclock.NewUserSubmitLock(),
		NodeEvalLock:       &nodeEvalFlight{inflight: map[string]chan struct{}{}},
		Base:               base,
		ClassNode:          repository.NewClassNodeRepository(base),
		NodeContent:        repository.NewNodeContentRepository(base),
		NodeProgress:       repository.NewNodeProgressRepository(base),
		NodeSubmission:     repository.NewNodeSubmissionRepository(base),
		AISession:          repository.NewAISessionRepository(base),
		AIMessage:          repository.NewAIMessageRepository(base),
		AISearchReference:  repository.NewAISearchReferenceRepository(base),
		User:               repository.NewUserRepository(base),
		ClassMember:        repository.NewClassMemberRepository(base),
		Class:              repository.NewClassRepository(base),
		CreationTask:       repository.NewCreationTaskRepository(base),
		CreationSubmission: repository.NewCreationSubmissionRepository(base),
		Resource:           repository.NewResourceRepository(base),
		AudioTranscription: repository.NewAudioTranscriptionRepository(base),
		Moment:             repository.NewMomentRepository(base),
		MomentImage:        repository.NewMomentImageRepository(base),
		MomentLike:         repository.NewMomentLikeRepository(base),
		MomentComment:      repository.NewMomentCommentRepository(base),
	}
	var ocrClient *ocr.Baidu
	if ocrCfg.APIKey != "" {
		ocrClient = ocr.NewBaidu(ocrCfg.APIKey, ocrCfg.SecretKey)
	}
	var sttClient *stt.Baidu
	if sttCfg.APIKey != "" {
		sttClient = stt.NewBaidu(sttCfg.APIKey, sttCfg.SecretKey)
	}
	var ttsClient *tts.Baidu
	if ttsCfg.APIKey != "" {
		ttsClient = tts.NewBaidu(ttsCfg.APIKey, ttsCfg.SecretKey)
	}

	svcs := &Services{
		WriteThoughts:    NewWriteThoughtsService(repos, llmClient, prompts),
		InitialInsight:   NewInitialInsightService(repos, llmClient, prompts),
		CulturalStyle:    NewCulturalStyleService(repos, llmClient, prompts),
		NodeState:        NewNodeStateService(repos),
		Account:          NewAccountService(repos, jwtCfg.Secret, jwtCfg.ExpireHours),
		Moment:           NewMomentService(repos),
		HeritageCultural: NewHeritageCulturalService(repos, llmClient, prompts),
		Upload:           NewUploadService(repos, uploadDir, storageURL, ocrClient, sttClient, ttsClient),
		ZhaoZhouQiao:     NewZhaoZhouQiaoService(repos, llmClient, prompts),
		WenMingZhongWai:  NewWenMingZhongWaiService(repos, llmClient, prompts),
	}
	if llmClient != nil {
		svcs.AI = NewAIService(repos, llmClient, prompts)
	}
	// Report 依赖 svcs（复用各节点 GetState 组装学习过程数据），须最后创建
	svcs.Report = NewReportService(repos, llmClient, prompts, svcs)
	return svcs
}

// NodeStateService 各课程子模块的节点参数与状态业务。
// 各子模块的 params 结构和 state 方法按课程拆分到独立文件：
//   - zhaozhouqiao.go: 赵州桥 GetParams/GetState
//   - wenmingzhongwai.go: 文明中外 GetParams/GetState
//   - cultural_style.go: 文化采风（独立 CulturalStyleService，含完整交互点链路）
type NodeStateService struct {
	repos *Repositories
}

// NewNodeStateService 创建 NodeStateService。
func NewNodeStateService(repos *Repositories) *NodeStateService {
	return &NodeStateService{repos: repos}
}

// GetVideoIntro 节点引导视频数据。
// TODO: 新版 api 中由 openapi 定义，待确认接口后实现。
func (s *NodeStateService) GetVideoIntro(ctx context.Context, classID, nodeID int64) (*api.Video, error) {
	return nil, ErrNotImplemented
}

// trimCodeFence 去掉 AI 输出中可能包裹的 ```json ... ``` 围栏。
func trimCodeFence(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 7 && (s[:3] == "```" || s[:3] == "~~~") {
		// 去掉第一行围栏
		if idx := strings.IndexAny(s, "\n"); idx >= 0 {
			s = s[idx+1:]
		}
		// 去掉末尾围栏
		s = strings.TrimSpace(s)
		if len(s) >= 3 && (s[len(s)-3:] == "```" || s[len(s)-3:] == "~~~") {
			s = s[:len(s)-3]
		}
	}
	return strings.TrimSpace(s)
}

// callAIWithJSON 通用方法：将 input 序列化 → 调 LLM → 去围栏 → 反序列化到 output。
// prompt 为 SystemPrompt，input 为 user 消息（JSON 序列化后发送），output 为结果反序列化目标。
func callAIWithJSON(ctx context.Context, llm ai.LLMClient, prompt string, input, output interface{}) error {
	if prompt == "" {
		return fmt.Errorf("prompt not configured")
	}
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("marshal ai input: %w", err)
	}
	resp, err := llm.Chat(ctx, &ai.ChatRequest{
		Messages: []ai.Message{
			{Role: "user", Content: string(inputJSON)},
		},
		MaxTokens:    2048,
		Temperature:  0.7,
		SystemPrompt: prompt,
	})
	if err != nil {
		return fmt.Errorf("llm chat: %w", err)
	}
	content := trimCodeFence(resp.Content)
	if err := json.Unmarshal([]byte(content), output); err != nil {
		return fmt.Errorf("parse ai result: %w", err)
	}
	return nil
}

// loadNodeParams 读取 node_contents.params_json 并解析到 target。
// 节点内容不存在或 params_json 为空时返回 ErrNodeContentNotConfigured。
func loadNodeParams(ctx context.Context, repos *Repositories, nodeID int64, target interface{}) error {
	nc, err := repos.NodeContent.GetByNodeID(ctx, nodeID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNodeContentNotConfigured
		}
		return fmt.Errorf("load node content: %w", err)
	}
	if len(nc.ParamsJSON) == 0 {
		return ErrNodeContentNotConfigured
	}
	if err := json.Unmarshal(nc.ParamsJSON, target); err != nil {
		return fmt.Errorf("parse params_json: %w", err)
	}
	return nil
}

// getMaxErrors 获取最大错误次数，默认 3。
func getMaxErrors(n int) int {
	if n <= 0 {
		return 3
	}
	return n
}

// isValidSubmitType 校验提交类型枚举，防止非法类型（如 type="foo"）绕过空输入/上限/幂等校验。
func isValidSubmitType(t string) bool {
	return t == "submit" || t == "get-answer"
}

// strPtrIfSet 空串返回 nil，非空返回指针。
func strPtrIfSet(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// SubmitWithDuration 一条按提交时间升序排列的 submit 记录及其作答耗时（秒）。
type SubmitWithDuration struct {
	Sub      model.NodeSubmission
	Duration int // 该次作答耗时（秒）
}

// isKeyCompletion 判定某条 submit 记录是否为"关键完成记录"：
// 该题/该空答对那次（IsPassed=true），或未答对但已完成的那次（IsCompleted=true，
// 即 get-answer 揭示答案 / 达到错误上限后自动填入答案），作为该段时间基准的依据。
func isKeyCompletion(sub model.NodeSubmission) bool {
	return (sub.IsPassed != nil && *sub.IsPassed) || (sub.IsCompleted != nil && *sub.IsCompleted)
}

// ComputeSegmentDurations 计算用户某节点每条 submit 记录的作答耗时（秒），供各节点 AI 评价等复用。
//
// 规则（分段基准累计耗时）：
//   - 仅统计 SubmitType == "submit" 的记录，get-answer 等不计入；
//   - segments 按作答顺序给出各分段键（写感想按题 q1/q2/q3，初步感悟按空 qid:blankId）；
//     keyOf 返回每条记录所属分段键，isKey 判定该记录是否为"关键完成记录"；
//   - 每段 keyAt = 该段内 isKey 记录中 submitted_at 最晚的一条；该段无关键完成记录时，
//     keyAt 回退为该段最后一条 submit 记录；
//   - 段 i 的起始基准 base = entryTime（i==0）或 segments[i-1].keyAt；
//     乱序作答（学生先答了本段、上一段尚未完成）时，理论基准会晚于本段最早提交，
//     此时回退到节点进入时间 entryTime 作为基准，避免耗时被算成 0 秒；
//   - 每条记录的 Duration = 该记录 submitted_at − 所属段起始基准（秒，负数取 0），
//     因此每段"完成那次"的 Duration 恰好 = 本段完成时间 − 上一段完成时间（首段 = 完成时间 − entryTime）；
//   - 返回值 total 为末条 submit 的 submitted_at − entryTime（整节点总用时，秒）。
func ComputeSegmentDurations(subs []model.NodeSubmission, entryTime time.Time, segments []string,
	keyOf func(sub model.NodeSubmission) string, isKey func(sub model.NodeSubmission) bool) ([]SubmitWithDuration, int) {
	// 1. 仅保留 submit 记录并按提交时间升序
	sorted := make([]model.NodeSubmission, 0, len(subs))
	for _, s := range subs {
		if s.SubmitType == "submit" {
			sorted = append(sorted, s)
		}
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].SubmittedAt.Before(sorted[j].SubmittedAt) })

	// 2. 计算每段关键完成时间点 keyAt（该段 isKey 记录中 submitted_at 最晚的一条）
	segIndex := make(map[string]int, len(segments))
	for i, k := range segments {
		segIndex[k] = i
	}
	keyAt := make(map[string]time.Time, len(segments))
	lastBySeg := make(map[string]time.Time, len(segments))  // 每段最后一条 submit 时间（兜底）
	firstBySeg := make(map[string]time.Time, len(segments)) // 每段最早一条 submit 时间（乱序防御用）
	for _, s := range sorted {
		k := keyOf(s)
		if k == "" {
			continue
		}
		if _, ok := segIndex[k]; !ok {
			continue
		}
		if t, ok := firstBySeg[k]; !ok || s.SubmittedAt.Before(t) {
			firstBySeg[k] = s.SubmittedAt
		}
		lastBySeg[k] = s.SubmittedAt
		if isKey(s) {
			keyAt[k] = s.SubmittedAt
		}
	}
	// 无关键完成记录的段，回退到该段最后一条提交
	for k := range segIndex {
		if _, ok := keyAt[k]; !ok {
			if t, ok := lastBySeg[k]; ok {
				keyAt[k] = t
			}
		}
	}

	// 3. 计算每段起始基准 base
	base := make(map[string]time.Time, len(segments))
	for i, k := range segments {
		var b time.Time
		if i == 0 {
			b = entryTime
		} else {
			b = keyAt[segments[i-1]]
		}
		// 乱序作答防御：若理论基准晚于该段自己的最早提交（学生先完成了本段、上一段还没完成），
		// 无法获得更精确的本段起点，回退到节点进入时间（entryTime 不可用时用本段最早提交），
		// 保证该段耗时不会因负数截断而恒为 0 秒。
		if first, ok := firstBySeg[k]; ok && !first.IsZero() && !b.IsZero() && b.After(first) {
			if entryTime.IsZero() {
				b = first
			} else {
				b = entryTime
			}
		}
		base[k] = b
	}

	// 4. 逐条计算耗时并求总用时
	out := make([]SubmitWithDuration, 0, len(sorted))
	var last time.Time
	for _, s := range sorted {
		last = s.SubmittedAt // 升序排列的末条即最后一条 submit
		d := 0
		if b, ok := base[keyOf(s)]; ok && !b.IsZero() && !s.SubmittedAt.IsZero() {
			if dd := int(s.SubmittedAt.Sub(b).Seconds()); dd > 0 {
				d = dd
			}
		}
		out = append(out, SubmitWithDuration{Sub: s, Duration: d})
	}

	total := 0
	if !entryTime.IsZero() && !last.IsZero() {
		if dd := int(last.Sub(entryTime).Seconds()); dd > 0 {
			total = dd
		}
	}
	return out, total
}

// Repositories 聚合 service 层所需的 repository。
type Repositories struct {
	// SubmitLock 按用户串行化异步提交（同一用户同一时间只允许一个提交），不同用户互不影响。
	SubmitLock *synclock.UserSubmitLock
	// MomentLock 朋友圈写操作的按用户串行化锁（发布/点赞/取消点赞/评论），
	// 与学习提交的 SubmitLock 相互独立，互不阻塞。
	MomentLock *synclock.UserSubmitLock
	// NodeEvalLock 节点综合评价生成的单飞锁（报告路径与提交预热共用），
	// 同一 (nodeID, userID) 同一时间只允许一次 AI 生成，避免并发重复调用。
	NodeEvalLock       *nodeEvalFlight
	Base               *repository.Base
	ClassNode          *repository.ClassNodeRepository
	NodeContent        *repository.NodeContentRepository
	NodeProgress       *repository.NodeProgressRepository
	NodeSubmission     *repository.NodeSubmissionRepository
	AISession          *repository.AISessionRepository
	AIMessage          *repository.AIMessageRepository
	AISearchReference  *repository.AISearchReferenceRepository
	User               *repository.UserRepository
	ClassMember        *repository.ClassMemberRepository
	Class              *repository.ClassRepository
	CreationTask       *repository.CreationTaskRepository
	CreationSubmission *repository.CreationSubmissionRepository
	Resource           *repository.ResourceRepository
	AudioTranscription *repository.AudioTranscriptionRepository
	Moment             *repository.MomentRepository
	MomentImage        *repository.MomentImageRepository
	MomentLike         *repository.MomentLikeRepository
	MomentComment      *repository.MomentCommentRepository
}

// RunInTx 在数据库事务中执行 fn，供需要跨表原子写（如提交记录 + 进度计数）的业务使用。
func (r *Repositories) RunInTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	return r.Base.RunInTx(ctx, fn)
}

// EnsureClassMember 防御性校验：确认 userID 是 classID 班级的成员。
// 防止任意登录用户凭猜测的 classID/nodeID 越权访问他人班级数据（IDOR）。
func (r *Repositories) EnsureClassMember(ctx context.Context, classID, userID int64) error {
	if userID <= 0 {
		return ErrClassMemberNotFound
	}
	_, err := r.ClassMember.GetByClassAndUser(ctx, classID, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrClassMemberNotFound
		}
		return err
	}
	return nil
}
