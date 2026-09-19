package service

import (
	"testing"
	"time"

	"zhonghuawenhua_backend/internal/model"
)

// testSub 构造一条 submit 记录。
func testSub(questionID string, at time.Time, passed, completed bool) model.NodeSubmission {
	p := &passed
	c := &completed
	return model.NodeSubmission{
		NodeID:      1,
		UserID:      1,
		QuestionID:  &questionID,
		SubmitType:  "submit",
		SubmittedAt: at,
		IsPassed:    p,
		IsCompleted: c,
	}
}

func TestComputeSegmentDurations_InOrder(t *testing.T) {
	entry := time.Date(2026, 8, 21, 9, 0, 0, 0, time.Local)
	q1 := testSub("q1", entry.Add(60*time.Second), true, true)
	q2 := testSub("q2", entry.Add(120*time.Second), true, true)
	q3 := testSub("q3", entry.Add(180*time.Second), true, true)

	segments := []string{"q1", "q2", "q3"}
	keyOf := func(sub model.NodeSubmission) string {
		if sub.QuestionID != nil {
			return *sub.QuestionID
		}
		return ""
	}
	entries, total := ComputeSegmentDurations([]model.NodeSubmission{q1, q2, q3}, entry, segments, keyOf, isKeyCompletion)
	if total != 180 {
		t.Errorf("total = %d, want 180", total)
	}
	want := []int{60, 60, 60}
	for i, e := range entries {
		if e.Duration != want[i] {
			t.Errorf("entry[%d] duration = %d, want %d", i, e.Duration, want[i])
		}
	}
}

// 乱序作答（先答 q2 再答 q1）：q2 的耗时当前会被算成 0（负数被截断），应为相对自身的合理耗时。
func TestComputeSegmentDurations_OutOfOrder(t *testing.T) {
	entry := time.Date(2026, 8, 21, 9, 0, 0, 0, time.Local)
	q2 := testSub("q2", entry.Add(60*time.Second), true, true)
	q1 := testSub("q1", entry.Add(120*time.Second), true, true)
	q3 := testSub("q3", entry.Add(180*time.Second), true, true)

	segments := []string{"q1", "q2", "q3"}
	keyOf := func(sub model.NodeSubmission) string {
		if sub.QuestionID != nil {
			return *sub.QuestionID
		}
		return ""
	}
	entries, _ := ComputeSegmentDurations([]model.NodeSubmission{q2, q1, q3}, entry, segments, keyOf, isKeyCompletion)
	// q2 完成于 entry+60s，其耗时不应为 0
	for _, e := range entries {
		if e.Sub.QuestionID != nil && *e.Sub.QuestionID == "q2" && e.Duration == 0 {
			t.Errorf("q2 (completed at entry+60s) duration = 0, want > 0 (out-of-order bug)")
		}
	}
}

// 某题多次提交：前一次答错、后一次答对，答对那次耗时应为正。
func TestComputeSegmentDurations_MultiAttempt(t *testing.T) {
	entry := time.Date(2026, 8, 21, 9, 0, 0, 0, time.Local)
	q1w := testSub("q1", entry.Add(30*time.Second), false, false)
	q1p := testSub("q1", entry.Add(90*time.Second), true, true)
	q2p := testSub("q2", entry.Add(150*time.Second), true, true)

	segments := []string{"q1", "q2"}
	keyOf := func(sub model.NodeSubmission) string {
		if sub.QuestionID != nil {
			return *sub.QuestionID
		}
		return ""
	}
	entries, _ := ComputeSegmentDurations([]model.NodeSubmission{q1w, q1p, q2p}, entry, segments, keyOf, isKeyCompletion)
	if len(entries) != 3 {
		t.Fatalf("entries len = %d, want 3", len(entries))
	}
	// q1 答对那次（index 1）耗时应为 90s
	if entries[1].Duration != 90 {
		t.Errorf("q1 passed duration = %d, want 90", entries[1].Duration)
	}
	// q2（index 2）完成于 150s，基准为 q1 完成 90s → 60s
	if entries[2].Duration != 60 {
		t.Errorf("q2 duration = %d, want 60", entries[2].Duration)
	}
}
