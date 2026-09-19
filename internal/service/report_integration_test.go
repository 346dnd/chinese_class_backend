package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"zhonghuawenhua_backend/internal/config"
	"zhonghuawenhua_backend/internal/model"
	"zhonghuawenhua_backend/internal/repository"
	"zhonghuawenhua_backend/pkg/db"
)

// newTestRepos 连接本地测试库构造 Repositories。
func newTestRepos(t *testing.T) *Repositories {
	t.Helper()
	cfg := config.DatabaseConfig{
		Host:     "localhost",
		Port:     5432,
		User:     "postgres",
		Password: "123456",
		Name:     "zhonghuawenhua",
		SSLMode:  "disable",
		MaxConns: 5,
	}
	pool, err := db.NewPostgres(context.Background(), cfg)
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}
	t.Cleanup(pool.Close)
	base := repository.NewBase(pool)
	ensureTestUser(t, base)
	return &Repositories{
		Base:           base,
		ClassNode:      repository.NewClassNodeRepository(base),
		NodeContent:    repository.NewNodeContentRepository(base),
		NodeProgress:   repository.NewNodeProgressRepository(base),
		NodeSubmission: repository.NewNodeSubmissionRepository(base),
		User:           repository.NewUserRepository(base),
		NodeEvalLock:   &nodeEvalFlight{inflight: map[string]chan struct{}{}},
	}
}

// ensureTestUser 创建集成测试专用的用户（node_submissions 有 user_id 外键约束）。
// 结束后清理该用户及其关联数据，避免污染测试库。
func ensureTestUser(t *testing.T, base *repository.Base) {
	t.Helper()
	const uid int64 = 999999
	if _, err := base.Pool.Exec(context.Background(),
		`INSERT INTO users (id, code, username, password_hash, role, real_name)
		 VALUES ($1, 'tester', 'tester', 'x', 'student', '测试同学')
		 ON CONFLICT (id) DO NOTHING`, uid); err != nil {
		t.Fatalf("create test user: %v", err)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = base.Pool.Exec(ctx, `DELETE FROM node_submissions WHERE user_id = $1`, uid)
		_, _ = base.Pool.Exec(ctx, `DELETE FROM node_progress WHERE user_id = $1`, uid)
		_, _ = base.Pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, uid)
	})
}

// insertTestSub 插入一条提交记录并返回 id。
func insertTestSub(t *testing.T, r *Repositories, userID, nodeID int64, qid, submitType string, payload map[string]string, passed, completed, revealed bool) {
	t.Helper()
	pb, _ := json.Marshal(payload)
	code := "t" + time.Now().Format("150405.000000000") + qid
	sub := &model.NodeSubmission{
		ClassID:      127,
		NodeID:       nodeID,
		UserID:       userID,
		NodeType:     model.NodeTypeWriteThoughts,
		QuestionID:   &qid,
		SubmitType:   submitType,
		PayloadJSON:  pb,
		SubmittedAt:  time.Now(),
		IsProcessing: false,
		Code:         code,
	}
	if submitType != "get-answer" {
		sub.IsPassed = &passed
	}
	if completed {
		sub.IsCompleted = &completed
	}
	sub.Revealed = revealed
	if _, err := r.NodeSubmission.Create(context.Background(), sub); err != nil {
		t.Fatalf("insert submission: %v", err)
	}
}

func TestBuildNodeStats_WriteThoughts_Completed(t *testing.T) {
	r := newTestRepos(t)
	ctx := context.Background()

	// 使用集成测试专用用户（见 ensureTestUser），避免影响现有数据
	userID := int64(999999)
	const nodeID int64 = 17

	insertTestSub(t, r, userID, nodeID, "q1", "submit", map[string]string{"input": "蔡伦改进造纸术"}, true, true, false)
	insertTestSub(t, r, userID, nodeID, "q2", "submit", map[string]string{"input": "赵州桥很坚固"}, true, true, false)
	insertTestSub(t, r, userID, nodeID, "q3", "submit", map[string]string{"input": "街市很热闹"}, true, true, false)

	stats := buildNodeStats(ctx, r, model.NodeTypeWriteThoughts, nodeID, userID)
	if !stats.Completed {
		t.Errorf("node should be Completed after all 3 questions passed, got Completed=%v", stats.Completed)
	}
	if stats.Points != 3 {
		t.Errorf("Points = %d, want 3", stats.Points)
	}
	t.Logf("Completed=%v Points=%d TotalTime=%d records=%d", stats.Completed, stats.Points, stats.TotalTime, len(stats.Input.Records))
	for _, rec := range stats.Input.Records {
		t.Logf("  %s attempt=%d pass=%v time=%ds", rec.SegmentTitle, rec.AttemptIndex, rec.IsPass, rec.TimeCost)
	}
}

func TestBuildNodeStats_WenMingZhongWai_Completed(t *testing.T) {
	r := newTestRepos(t)
	ctx := context.Background()

	const userID int64 = 999999
	const nodeID int64 = 21

	// 三次提交，前两次有错误，最后一次全部答对
	sub1 := map[string]string{"r2": "错误答案1", "r3": "错误答案2", "r4": "错误答案3"}
	sub2 := map[string]string{"r2": "热闹", "r3": "画上的街市可热闹了", "r4": "错"}
	sub3 := map[string]string{"r2": "热闹", "r3": "画上的街市可热闹了", "r4": "街上有挂着各种招牌的店铺。走在街上的，是来来往往、形态各异的人：有的骑着马，有的挑着担，有的赶着毛驴，有的推着独轮车，有的悠闲地在街上溜达。"}

	// 手工插入三条 submit 记录
	now := time.Now()
	insertWMZ(t, r, userID, nodeID, "q1", sub1, now.Add(-6*time.Minute), false)
	insertWMZ(t, r, userID, nodeID, "q1", sub2, now.Add(-3*time.Minute), false)
	insertWMZ(t, r, userID, nodeID, "q1", sub3, now, true)

	stats := buildNodeStats(ctx, r, model.NodeTypeWenMingZhongWai, nodeID, userID)
	if !stats.Completed {
		t.Errorf("wenmingzhongwai node should be Completed after all rows solved, got Completed=%v", stats.Completed)
	}
	if stats.Points != 3 {
		t.Errorf("Points = %d, want 3", stats.Points)
	}
	if stats.Input == nil || stats.Input.WenMingZhongWai == nil {
		t.Fatalf("WenMingZhongWai data packet missing")
	}
	t.Logf("Completed=%v Points=%d TotalTime=%d records=%d accuracy=%v",
		stats.Completed, stats.Points, stats.TotalTime,
		len(stats.Input.WenMingZhongWai.StudentData.Records),
		stats.Input.WenMingZhongWai.StudentData.AccuracyRate)
}

func insertWMZ(t *testing.T, r *Repositories, userID, nodeID int64, qid string, answers map[string]string, at time.Time, passed bool) {
	t.Helper()
	pb, _ := json.Marshal(map[string]interface{}{"questionId": qid, "answers": answers})
	code := "wmz" + at.Format("150405") + qid
	sub := &model.NodeSubmission{
		ClassID:      127,
		NodeID:       nodeID,
		UserID:       userID,
		NodeType:     model.NodeTypeWenMingZhongWai,
		QuestionID:   &qid,
		SubmitType:   "submit",
		PayloadJSON:  pb,
		SubmittedAt:  at,
		IsProcessing: false,
		Code:         code,
	}
	sub.IsPassed = &passed
	if passed {
		sub.IsCompleted = &passed
	}
	if _, err := r.NodeSubmission.Create(context.Background(), sub); err != nil {
		t.Fatalf("insert wmz submission: %v", err)
	}
}
