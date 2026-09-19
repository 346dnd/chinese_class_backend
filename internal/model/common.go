// Package model 定义与数据库表结构对应的数据模型。
//
// 模型层职责：表示数据库行（DB row），与 internal/api 中的 DTO 分离。
// 通过 db tag 供 scany 扫描，通过 json tag 与 api DTO 对齐便于转换。
package model

// UserRole 用户角色枚举（对应 DB user_role）。
type UserRole string

const (
	UserRoleAdmin   UserRole = "admin"
	UserRoleTeacher UserRole = "teacher"
	UserRoleStudent UserRole = "student"
)

// Valid 校验枚举值是否合法。
func (e UserRole) Valid() bool {
	switch e {
	case UserRoleAdmin, UserRoleTeacher, UserRoleStudent:
		return true
	default:
		return false
	}
}

// NodeType 课程节点类型枚举（对应 DB node_type，新版 10 种）。
type NodeType string

const (
	NodeTypeWriteThoughts          NodeType = "write-thoughts"
	NodeTypeInitialInsight         NodeType = "initial-insight"
	NodeTypeCulturalStyle          NodeType = "cultural-style"
	NodeTypeCulturalStyleQuickSelect NodeType = "cultural-style-quick-select"
	NodeTypeCulturalStyleDragSort  NodeType = "cultural-style-drag-sort"
	NodeTypeZhaoZhouQiao           NodeType = "zhaozhouqiao"
	NodeTypeWenMingZhongWai        NodeType = "wenmingzhongwai"
	NodeTypeHeritageCultural       NodeType = "heritage-cultural"
	NodeTypeCreationWorkshop       NodeType = "creation-workshop"
	NodeTypeFolder                 NodeType = "folder"
)

// Valid 校验枚举值是否合法。
func (e NodeType) Valid() bool {
	switch e {
	case NodeTypeWriteThoughts, NodeTypeInitialInsight, NodeTypeCulturalStyle,
		NodeTypeCulturalStyleQuickSelect, NodeTypeCulturalStyleDragSort,
		NodeTypeZhaoZhouQiao, NodeTypeWenMingZhongWai, NodeTypeHeritageCultural,
		NodeTypeCreationWorkshop, NodeTypeFolder:
		return true
	default:
		return false
	}
}

// SceneType 场景类型枚举（对应 DB scene_type）。
type SceneType string

const (
	SceneTypeInclass    SceneType = "inclass"
	SceneTypeAfterclass SceneType = "afterclass"
)

// Valid 校验枚举值是否合法。
func (e SceneType) Valid() bool {
	switch e {
	case SceneTypeInclass, SceneTypeAfterclass:
		return true
	default:
		return false
	}
}

// CreationTaskType 创作工坊任务类型枚举（对应 DB creation_task_type）。
type CreationTaskType string

const (
	CreationTaskTypeGeneration CreationTaskType = "generation"
	CreationTaskTypePoemScripts CreationTaskType = "poemscripts"
	CreationTaskTypeShare      CreationTaskType = "share"
	CreationTaskTypeExpress    CreationTaskType = "express"
)

// Valid 校验枚举值是否合法。
func (e CreationTaskType) Valid() bool {
	switch e {
	case CreationTaskTypeGeneration, CreationTaskTypePoemScripts,
		CreationTaskTypeShare, CreationTaskTypeExpress:
		return true
	default:
		return false
	}
}

// CreationStatus 创作工坊提交状态枚举（对应 DB creation_status）。
type CreationStatus string

const (
	CreationStatusPending    CreationStatus = "pending"
	CreationStatusProcessing CreationStatus = "processing"
	CreationStatusSuccessful CreationStatus = "successful"
	CreationStatusPublished  CreationStatus = "published"
)

// Valid 校验枚举值是否合法。
func (e CreationStatus) Valid() bool {
	switch e {
	case CreationStatusPending, CreationStatusProcessing,
		CreationStatusSuccessful, CreationStatusPublished:
		return true
	default:
		return false
	}
}

// ChatFinishReason AI 聊天结束原因枚举（对应 DB chat_finish_reason）。
type ChatFinishReason string

const (
	ChatFinishReasonStop      ChatFinishReason = "stop"
	ChatFinishReasonLength    ChatFinishReason = "length"
	ChatFinishReasonToolCalls ChatFinishReason = "tool_calls"
)

// Valid 校验枚举值是否合法。
func (e ChatFinishReason) Valid() bool {
	switch e {
	case ChatFinishReasonStop, ChatFinishReasonLength, ChatFinishReasonToolCalls:
		return true
	default:
		return false
	}
}

// AudioTransStatus 语音转写状态枚举（对应 DB audio_trans_status）。
type AudioTransStatus string

const (
	AudioTransStatusProcessing AudioTransStatus = "processing"
	AudioTransStatusFinish     AudioTransStatus = "finish"
	AudioTransStatusFail       AudioTransStatus = "fail"
)

// Valid 校验枚举值是否合法。
func (e AudioTransStatus) Valid() bool {
	switch e {
	case AudioTransStatusProcessing, AudioTransStatusFinish, AudioTransStatusFail:
		return true
	default:
		return false
	}
}

// AIMessageRole AI 消息角色枚举（对应 DB ai_message_role）。
type AIMessageRole string

const (
	AIMessageRoleUser      AIMessageRole = "user"
	AIMessageRoleAssistant AIMessageRole = "assistant"
	AIMessageRoleSystem    AIMessageRole = "system"
)

// Valid 校验枚举值是否合法。
func (e AIMessageRole) Valid() bool {
	switch e {
	case AIMessageRoleUser, AIMessageRoleAssistant, AIMessageRoleSystem:
		return true
	default:
		return false
	}
}

// ResourceMediaType 资源媒体类型枚举（对应 DB resource_media_type）。
type ResourceMediaType string

const (
	ResourceMediaTypeImage ResourceMediaType = "image"
	ResourceMediaTypeAudio ResourceMediaType = "audio"
	ResourceMediaTypeVideo ResourceMediaType = "video"
)

// Valid 校验枚举值是否合法。
func (e ResourceMediaType) Valid() bool {
	switch e {
	case ResourceMediaTypeImage, ResourceMediaTypeAudio, ResourceMediaTypeVideo:
		return true
	default:
		return false
	}
}

// ActivityStatus 活动状态枚举（对应 DB activity_status）。
type ActivityStatus string

const (
	ActivityStatusNotStarted ActivityStatus = "NotStarted"
	ActivityStatusInProgress ActivityStatus = "InProgress"
	ActivityStatusFinished   ActivityStatus = "Finished"
)

// Valid 校验枚举值是否合法。
func (e ActivityStatus) Valid() bool {
	switch e {
	case ActivityStatusNotStarted, ActivityStatusInProgress, ActivityStatusFinished:
		return true
	default:
		return false
	}
}
