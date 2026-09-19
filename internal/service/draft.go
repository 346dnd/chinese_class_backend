package service

import (
	"context"
	"encoding/json"
	"fmt"
)

// 模块分区 key 常量，用于 node_progress.draft_json 顶层分区存储。
// 同一 (node_id, user_id) 下多个子模块的草稿按 key 分区，互不覆盖。
// 新版 schema node_id 全局唯一，不再依赖 class_id 区分。
const (
	DraftKeyWriteThoughts   = "write-thoughts"
	DraftKeyInitialInsight  = "initial-insight"
	DraftKeyZhaoZhouQiao    = "zhaozhouqiao"
	DraftKeyWenMingZhongWai = "wenmingzhongwai"
	DraftKeyCulturalStyle   = "cultural-style"
)

// loadDraftPartition 读取 node_progress.draft_json 中指定 moduleKey 分区的数据到 target。
// 进度不存在、分区不存在或解析失败时 target 不变，均返回 nil。
func loadDraftPartition(ctx context.Context, repos *Repositories, nodeID, userID int64, moduleKey string, target interface{}) error {
	prog, err := repos.NodeProgress.GetByNodeUser(ctx, nodeID, userID)
	if err != nil {
		return nil // 进度不存在 = 空草稿
	}
	if len(prog.DraftJSON) == 0 {
		return nil
	}
	var full map[string]json.RawMessage
	if err := json.Unmarshal(prog.DraftJSON, &full); err != nil {
		return nil // 无法解析为分区格式，视为空
	}
	part, ok := full[moduleKey]
	if !ok || len(part) == 0 {
		return nil
	}
	if target != nil {
		return json.Unmarshal(part, target)
	}
	return nil
}

// saveDraftPartition 将 partitionData 保存到 node_progress.draft_json 的指定 moduleKey 分区。
// 已有其他分区保持不变；进度不存在时自动创建。
// classID 仅写入新进度记录的归属字段，不参与唯一约束查询。
//
// 防御性要求：由 repository 在 SQL 层用 jsonb_set 原子合并（仅更新 draft_json 列），
// 避免"读-改-写"整行回写覆盖并发更新的 attempt_count / ai_evaluation_json 等字段（丢失更新）。
func saveDraftPartition(ctx context.Context, repos *Repositories, classID, nodeID, userID int64, moduleKey string, partitionData interface{}) error {
	partitionBytes, err := json.Marshal(partitionData)
	if err != nil {
		return fmt.Errorf("marshal partition data: %w", err)
	}
	if err := repos.NodeProgress.SetDraftPartition(ctx, classID, nodeID, userID, moduleKey, partitionBytes); err != nil {
		return fmt.Errorf("set draft partition: %w", err)
	}
	return nil
}
