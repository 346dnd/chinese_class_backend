package service

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"zhonghuawenhua_backend/internal/api"
	"zhonghuawenhua_backend/internal/model"
)

// 课程包特殊值（客户端写死的课程包 ID）。
const classDetailCourse = api.N4

// nodeTypeToKey 将节点类型映射为前端组件对应的节点 key。
// 无对应 key 的节点（如 folder）返回 false，调用方跳过。
func nodeTypeToKey(nt model.NodeType) (api.StuClassV1AvailableNodeKey, bool) {
	switch nt {
	case model.NodeTypeWriteThoughts:
		return api.WriteThoughts, true
	case model.NodeTypeInitialInsight:
		return api.InitialInsight, true
	case model.NodeTypeCulturalStyle:
		return api.CulturalStyle, true
	case model.NodeTypeZhaoZhouQiao:
		return api.Zhaozhouqiao, true
	case model.NodeTypeWenMingZhongWai:
		return api.Wenmingzhongwai, true
	case model.NodeTypeHeritageCultural:
		return api.HeritageCultural, true
	case model.NodeTypeCreationWorkshop:
		return api.CultureWorkshop, true
	}
	return "", false
}

// GetClassDetail 返回课堂详细信息：章节（sections）+ 章节下的节点列表。
// 结构约定：class_nodes 为树形，根节点（parent_id IS NULL）的子节点即章节（section），
// 章节的子节点即学习节点（node）。节点是否完成取 node_progress.completed。
func (s *NodeStateService) GetClassDetail(ctx context.Context, classID, userID int64) (*api.StuClassV1DetailResp, error) {
	if err := s.repos.EnsureClassMember(ctx, classID, userID); err != nil {
		return nil, err
	}
	all, err := s.repos.ClassNode.ListByClassID(ctx, classID)
	if err != nil {
		return nil, err
	}

	// 建立父子关系
	byParent := map[int64][]model.ClassNode{}
	rootID := int64(0)
	for _, n := range all {
		if n.ParentID == nil {
			rootID = n.ID
			continue
		}
		byParent[*n.ParentID] = append(byParent[*n.ParentID], n)
	}

	// 章节 = 根节点的子节点（无根时退化为所有顶层节点）
	sectionNodes := byParent[rootID]
	if rootID == 0 {
		for _, n := range all {
			if n.ParentID == nil {
				sectionNodes = append(sectionNodes, n)
			}
		}
	}

	sections := make([]api.StuClassV1Section, 0, len(sectionNodes))
	for _, sec := range sectionNodes {
		nodes := make([]api.StuClassV1Node, 0, len(byParent[sec.ID]))
		for _, child := range byParent[sec.ID] {
			key, ok := nodeTypeToKey(child.NodeType)
			if !ok {
				continue
			}
			enabled, completed, reentry := true, s.nodeCompleted(ctx, child.ID, userID), true
			nodes = append(nodes, api.StuClassV1Node{
				ID:           int(child.ID),
				Key:          key,
				Title:        child.Title,
				IsEnabled:    &enabled,
				IsCompleted:  &completed,
				AllowReentry: &reentry,
			})
		}
		sections = append(sections, api.StuClassV1Section{
			ID:    int(sec.ID),
			Name:  sec.Title,
			Nodes: nodes,
		})
	}

	return &api.StuClassV1DetailResp{
		Course:   classDetailCourse,
		Sections: sections,
	}, nil
}

// nodeCompleted 查询某节点对某用户是否已完成。
func (s *NodeStateService) nodeCompleted(ctx context.Context, nodeID, userID int64) bool {
	prog, err := s.repos.NodeProgress.GetByNodeUser(ctx, nodeID, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false
		}
		return false
	}
	return prog.Completed
}
