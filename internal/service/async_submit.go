package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5"

	"zhonghuawenhua_backend/internal/config"
	"zhonghuawenhua_backend/internal/model"
	"zhonghuawenhua_backend/pkg/ai"
)

// AsyncSubmitResult 异步提交处理完成后的最终回写结果。
type AsyncSubmitResult struct {
	IsPassed        bool
	IsCompleted     bool
	Revealed        bool
	Feedback        string
	ReferenceAnswer string
	// ResultJSON 可选的结构化回写内容（如文明中外的 errors/ending 列表），
	// 序列化后写入 node_submissions.result_json，供前端 DTO 还原（见 ToWenMingZhongWaiResp）。
	ResultJSON interface{}
}

// AsyncSubmitTask 一次异步提交任务的配置。
type AsyncSubmitTask struct {
	ClassID     int64
	NodeID      int64
	UserID      int64
	NodeType    model.NodeType
	QuestionID  string
	SubmitType  string
	PayloadJSON []byte

	// Process 在后台 goroutine 中执行节点特有的业务（如 AI 评分/生成参考答案），返回回写结果。
	Process func(ctx context.Context) (*AsyncSubmitResult, error)
	// FinalizeProgress 在结果回写后更新进度聚合字段（error_count/revealed/completed）。
	FinalizeProgress func(ctx context.Context, sub *model.NodeSubmission) error

	// LLM / Prompts 用于节点全部完成时异步生成并缓存 AI 综合评价（可选；为空则跳过）。
	LLM     ai.LLMClient
	Prompts config.PromptsConfig
}

// submitProcessTimeout 异步提交处理（含 AI 调用）的整体超时。
// 防御性要求：即使底层 AI 客户端卡住，也保证在超时后回写失败状态，
// 避免 is_processing 永久 true（前端永远"处理中"）。
const submitProcessTimeout = 60 * time.Second

// SubmitAsync 通用异步提交入口。
//
// 流程：创建 is_processing=true 的提交记录 → 同步更新进度基础字段（attempt_count/last_submit_id）
// → 启动后台 goroutine 执行 task.Process 得到结果并回写提交记录 → 调用 task.FinalizeProgress 更新进度聚合字段。
// 主流程立即返回 submitId，供前端通过 GetSubmission 轮询 is_processing 获取最终结果。
func SubmitAsync(ctx context.Context, repos *Repositories, task AsyncSubmitTask) (string, error) {
	// 按用户串行化：同一用户同一时间只允许一个异步提交，被占用时立即拒绝（ErrSubmitInProgress），
	// 避免用户快速连点刷出多个并发 AI 任务；不同用户各自独立提交槽，互不影响。
	if !repos.SubmitLock.TryAcquire(task.UserID) {
		return "", ErrSubmitInProgress
	}

	sub := &model.NodeSubmission{
		ClassID:      task.ClassID,
		NodeID:       task.NodeID,
		UserID:       task.UserID,
		NodeType:     task.NodeType,
		SubmitType:   task.SubmitType,
		PayloadJSON:  task.PayloadJSON,
		SubmittedAt:  time.Now(),
		IsProcessing: true,
	}
	// QuestionID 仅非空时赋值（如讲解优秀文化无题目维度，保持 question_id 为 NULL）
	if task.QuestionID != "" {
		sub.QuestionID = &task.QuestionID
	}
	// 计算本次提交耗时（距该用户该节点上一次提交的秒数，首条为 0），
	// 供学习过程/报告逐条展示（此前 node_submissions.duration 从未写入，相关 DTO 耗时恒为 0）。
	// 同一用户提交已被 SubmitLock 串行化，此处的读-算-写无并发竞态。
	if prev, err := repos.NodeSubmission.LatestSubmittedAt(ctx, task.UserID, task.NodeID); err == nil && !prev.IsZero() {
		if d := int(sub.SubmittedAt.Sub(prev).Seconds()); d > 0 {
			sub.Duration = &d
		}
	}

	var subID int64
	if err := repos.RunInTx(ctx, func(tx pgx.Tx) error {
		id, err := repos.NodeSubmission.CreateTx(ctx, tx, sub)
		if err != nil {
			return err
		}
		subID = id
		// 原子自增 attempt_count 并记录 last_submit_id，与提交插入同一事务，保证原子性且无并发竞态
		return repos.NodeProgress.BumpAttemptTx(ctx, tx, task.ClassID, task.NodeID, task.UserID, id)
	}); err != nil {
		// 落库失败提前返回，需释放已占用的提交槽，避免用户被永久锁死
		repos.SubmitLock.Release(task.UserID)
		return "", fmt.Errorf("create submission: %w", err)
	}
	sub.ID = subID

	// 后台 goroutine 执行节点业务并回写，主流程不阻塞
	go func() {
		defer repos.SubmitLock.Release(task.UserID) // 任务结束释放该用户提交槽
		defer func() {
			if r := recover(); r != nil {
				log.Printf("async submission process panic: %v", r)
				failSubmission(repos, sub)
			}
		}()

		res, err := func() (*AsyncSubmitResult, error) {
			processCtx, cancel := context.WithTimeout(context.Background(), submitProcessTimeout)
			defer cancel()
			return task.Process(processCtx)
		}()
		if err != nil {
			failSubmission(repos, sub)
			return
		}

		if res != nil {
			// 有回写内容（AI 评分/参考答案等）
			sub.IsProcessing = false
			sub.IsPassed = &res.IsPassed
			sub.IsCompleted = &res.IsCompleted
			sub.Revealed = res.Revealed
			if res.Feedback != "" {
				sub.Feedback = &res.Feedback
			}
			if res.ReferenceAnswer != "" {
				sub.ReferenceAnswer = model.StrPtrEmpty(res.ReferenceAnswer)
			}
			if res.ResultJSON != nil {
				// 结构化结果（errors/ending/referenceAnswer 等）写入 result_json
				if data, err := json.Marshal(res.ResultJSON); err == nil {
					sub.ResultJSON = data
				} else {
					log.Printf("async submission marshal result json failed: %v", err)
				}
			}
		} else {
			// Process 未返回回写内容：仅停止处理中状态
			sub.IsProcessing = false
		}
		// 回写提交记录：失败重试一次，仍失败则按失败处理，保证 is_processing 不会永久卡 true
		if err := updateSubmissionWithRetry(repos, sub); err != nil {
			log.Printf("async submission update failed: %v", err)
			failSubmission(repos, sub)
			return
		}

		if task.FinalizeProgress != nil {
			if err := task.FinalizeProgress(context.Background(), sub); err != nil {
				log.Printf("async submission finalize progress failed: %v", err)
			}
		}

		// 本次提交完成了某题目后，若整个节点已全部完成且尚无缓存评价，则异步生成 AI 综合评价并缓存
		if task.LLM != nil && sub.IsCompleted != nil && *sub.IsCompleted {
			maybeTriggerNodeEvaluation(context.Background(), repos, task.LLM, task.Prompts, task.NodeID, task.UserID, task.NodeType)
		}
	}()

	return sub.SubmitIDStr(), nil
}

// failSubmission 处理失败时回写提交记录：停止处理中状态，is_passed 保持 nil，
// 前端可据 isProcessing=false 且 isPassed=nil 判断处理失败。
func failSubmission(repos *Repositories, sub *model.NodeSubmission) {
	sub.IsProcessing = false
	msg := "AI 处理失败，请稍后重试"
	sub.Feedback = &msg
	if err := updateSubmissionWithRetry(repos, sub); err != nil {
		log.Printf("async submission fail update: %v", err)
	}
}

// updateSubmissionWithRetry 回写提交记录，失败自动重试一次。
// 防御性要求：临时性 DB 抖动不丢回写，最大程度避免 is_processing 永久卡 true。
func updateSubmissionWithRetry(repos *Repositories, sub *model.NodeSubmission) error {
	for attempt := 0; attempt < 2; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), submitProcessTimeout)
		err := repos.NodeSubmission.Update(ctx, sub)
		cancel()
		if err == nil {
			return nil
		}
	}
	return fmt.Errorf("update submission after retries")
}
