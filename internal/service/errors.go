package service

import "net/http"

// BizError 业务错误：携带 HTTP 状态码与稳定业务码，供 handler 统一映射响应。
// 前端应按 Code 分支（经响应 Tip 字段透出），而非匹配 Msg 文本。
type BizError struct {
	Status int    // HTTP 状态码
	Code   string // 稳定业务码，如 "NODE_NOT_FOUND"
	Msg    string // 人类可读的错误描述
}

// Error 实现 error 接口。
func (e *BizError) Error() string { return e.Msg }

// newBiz 构造业务错误。
func newBiz(status int, code, msg string) *BizError {
	return &BizError{Status: status, Code: code, Msg: msg}
}

// 业务错误。handler 层统一通过 respondError 按 BizError.Status / Code 映射到 HTTP 响应。
var (
	// 节点相关
	ErrNodeNotFound             = newBiz(http.StatusBadRequest, "NODE_NOT_FOUND", "node not found")
	ErrNodeNotInClass           = newBiz(http.StatusBadRequest, "NODE_NOT_IN_CLASS", "node does not belong to the specified class")
	ErrNodeTypeMismatch         = newBiz(http.StatusBadRequest, "NODE_TYPE_MISMATCH", "node type mismatch")
	ErrNodeContentNotConfigured = newBiz(http.StatusBadRequest, "NODE_CONTENT_NOT_CONFIGURED", "node content not configured")
	ErrQuestionNotFound         = newBiz(http.StatusBadRequest, "QUESTION_NOT_FOUND", "question not found in node content")
	ErrEmptyInput               = newBiz(http.StatusBadRequest, "EMPTY_INPUT", "input must not be empty")
	ErrSubmitLimitReached       = newBiz(http.StatusBadRequest, "SUBMIT_LIMIT_REACHED", "已达本题最大提交次数，无法继续提交")
	ErrInvalidSubmitType        = newBiz(http.StatusBadRequest, "INVALID_SUBMIT_TYPE", "submit type 仅支持 submit/get-answer")
	ErrSubmitInProgress         = newBiz(http.StatusConflict, "SUBMIT_IN_PROGRESS", "正在批改上一提交，请稍候再试")
	ErrClassMemberNotFound      = newBiz(http.StatusForbidden, "CLASS_MEMBER_NOT_FOUND", "user is not a member of the class")

	// 综合评价相关
	ErrEvaluationNotReady   = newBiz(http.StatusBadRequest, "EVALUATION_NOT_READY", "你还没有完成该任务，请继续完成后再查看综合评价")
	ErrEvaluationGenerating = newBiz(http.StatusConflict, "EVALUATION_GENERATING", "AI 综合评价生成中，请稍后刷新")

	// 账号相关
	ErrInvalidCredentials = newBiz(http.StatusUnauthorized, "ACCOUNT_INVALID_CREDENTIALS", "invalid username or password")
	ErrUserDisabled       = newBiz(http.StatusUnauthorized, "ACCOUNT_USER_DISABLED", "user is disabled")
	ErrUserNotFound       = newBiz(http.StatusUnauthorized, "ACCOUNT_USER_NOT_FOUND", "user not found")

	// 创作工坊相关
	ErrCreationTaskNotFound       = newBiz(http.StatusBadRequest, "CREATION_TASK_NOT_FOUND", "creation task not found")
	ErrCreationSubmissionNotFound = newBiz(http.StatusBadRequest, "CREATION_SUBMISSION_NOT_FOUND", "creation submission not found")
	ErrCreationNotOwned           = newBiz(http.StatusForbidden, "CREATION_NOT_OWNED", "creation does not belong to the user")

	// 朋友圈相关
	ErrMomentNotFound            = newBiz(http.StatusBadRequest, "MOMENT_NOT_FOUND", "moment not found")
	ErrMomentNotOwned            = newBiz(http.StatusForbidden, "MOMENT_NOT_OWNED", "moment does not belong to the user")
	ErrMomentContentEmpty        = newBiz(http.StatusBadRequest, "MOMENT_CONTENT_EMPTY", "moment content must not be empty")
	ErrCommentNotFound           = newBiz(http.StatusBadRequest, "COMMENT_NOT_FOUND", "comment not found")
	ErrCommentEmpty              = newBiz(http.StatusBadRequest, "COMMENT_EMPTY", "comment text must not be empty")
	ErrMomentAlreadyLiked        = newBiz(http.StatusBadRequest, "MOMENT_ALREADY_LIKED", "moment already liked by user")
	ErrMomentNotLiked            = newBiz(http.StatusBadRequest, "MOMENT_NOT_LIKED", "moment not liked by user yet")
	ErrMomentOperationInProgress = newBiz(http.StatusConflict, "MOMENT_OPERATION_IN_PROGRESS", "操作过于频繁，请稍候再试")
	ErrResourceNotFound          = newBiz(http.StatusBadRequest, "RESOURCE_NOT_FOUND", "resource not found")

	// AI 会话相关
	ErrAISessionNotFound     = newBiz(http.StatusBadRequest, "AI_SESSION_NOT_FOUND", "ai session not found")
	ErrAISessionAccessDenied = newBiz(http.StatusForbidden, "AI_SESSION_ACCESS_DENIED", "ai session access denied")

	// 工具-文件相关
	ErrInvalidMediaType = newBiz(http.StatusBadRequest, "INVALID_MEDIA_TYPE", "media_type 仅支持 image/audio/video")
	ErrFileTooLarge     = newBiz(http.StatusBadRequest, "FILE_TOO_LARGE", "文件大小超过上限")

	// 工具-OCR 相关
	ErrOCRNotConfigured = newBiz(http.StatusBadRequest, "OCR_NOT_CONFIGURED", "百度 OCR 未配置")
	ErrNotImage         = newBiz(http.StatusBadRequest, "NOT_IMAGE", "资源不是图片")

	// 工具-语音识别相关
	ErrSTTNotConfigured       = newBiz(http.StatusBadRequest, "STT_NOT_CONFIGURED", "百度语音识别未配置")
	ErrNotAudio               = newBiz(http.StatusBadRequest, "NOT_AUDIO", "资源不是音频")
	ErrUnsupportedAudioFormat = newBiz(http.StatusBadRequest, "UNSUPPORTED_AUDIO_FORMAT", "不支持的音频格式（百度短语音识别仅支持 pcm/wav/amr/m4a，16k 采样率单声道）")

	// 工具-语音合成相关
	ErrTTSNotConfigured = newBiz(http.StatusBadRequest, "TTS_NOT_CONFIGURED", "百度语音合成未配置")

	// 通用：接口暂未实现（适配新版 API schema 进行中）
	ErrNotImplemented = newBiz(http.StatusNotImplemented, "NOT_IMPLEMENTED", "not implemented")
)
