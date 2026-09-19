// Package handler 实现 api.ServerInterface，本文件为基础设施骨架，
// 未实现的业务方法暂时返回 501 Not Implemented。
package handler

import (
	"fmt"

	"github.com/gin-gonic/gin"

	"zhonghuawenhua_backend/internal/api"
	"zhonghuawenhua_backend/internal/service"
	"zhonghuawenhua_backend/pkg/httpresp"
)

// Server 实现 api.ServerInterface。
type Server struct {
	services *service.Services
}

// NewServer 创建 Server 实例，注入业务 service 聚合。
func NewServer(services *service.Services) *Server {
	return &Server{services: services}
}

var _ api.ServerInterface = (*Server)(nil)

// --- AI 对话 ---

// chatChoice 与 api.ChatResp.Choices 内联匿名结构体保持一致（类型别名可直接赋值）。
type chatChoice = struct {
	FinishReason *api.ChatRespChoicesFinishReason `json:"finish_reason,omitempty"`
	Index        *int                             `json:"index,omitempty"`
	Message      struct {
		Content string `json:"content"`
		Role    string `json:"role"`
	} `json:"message"`
}

func (s *Server) PostAPIAiChatStreamSession(c *gin.Context, session string) {
	s.PostAPIAiChatSession(c, session)
}

func (s *Server) PostAPIAiChatSession(c *gin.Context, session string) {
	if s.services.AI == nil {
		httpresp.BadRequest(c, "ai service not configured", "")
		return
	}
	userID, ok := currentUserID(c)
	if !ok {
		httpresp.Unauthorized(c, "missing user")
		return
	}
	var req api.ChatReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresp.BadRequest(c, "invalid request body", err.Error())
		return
	}
	content := ""
	if req.Promot != nil && *req.Promot != "" {
		content = *req.Promot
	} else if req.Messages != nil && len(*req.Messages) > 0 {
		last := (*req.Messages)[len(*req.Messages)-1]
		content = last.Content
	}
	if content == "" {
		httpresp.BadRequest(c, "promot or messages required", "")
		return
	}
	sessionID := int64(0)
	if session != "" && session != "new" {
		if sid, err := parseSessionID(session); err == nil {
			sessionID = sid
		}
	}
	postReq := &service.PostAIChatReq{
		Content:      content,
		SessionID:    sessionID,
		SystemPrompt: c.Query("system_prompt"), // query param 优先
	}

	// 从 query 参数读取 node_id 和 class_id（可选）
	if nid := c.Query("node_id"); nid != "" {
		if id, err := parseSessionID(nid); err == nil {
			postReq.NodeID = id
		}
	}
	if cid := c.Query("class_id"); cid != "" {
		if id, err := parseSessionID(cid); err == nil {
			postReq.ClassID = id
		}
	}
	result, err := s.services.AI.Chat(c.Request.Context(), userID, postReq)
	if err != nil {
		httpresp.InternalError(c, "ai chat failed", err.Error())
		return
	}
	lastMsg := result.Messages[len(result.Messages)-1]
	chatResp := api.ChatResp{
		ID: fmt.Sprintf("chat-%d", result.SessionID),
		Choices: []chatChoice{
			{
				Message: struct {
					Content string `json:"content"`
					Role    string `json:"role"`
				}{Role: "assistant", Content: lastMsg.Content},
			},
		},
	}
	httpresp.OK(c, chatResp)
}

func (s *Server) GetAPIAiContext(c *gin.Context, params api.GetAPIAiContextParams) {
	if s.services.AI == nil {
		httpresp.BadRequest(c, "ai service not configured", "")
		return
	}
	userID, ok := currentUserID(c)
	if !ok {
		httpresp.Unauthorized(c, "missing user")
		return
	}
	sessionID, err := parseSessionID(params.Session)
	if err != nil {
		httpresp.BadRequest(c, "invalid session id", "")
		return
	}
	result, err := s.services.AI.GetMessages(c.Request.Context(), userID, sessionID)
	if err != nil {
		respondError(c, err)
		return
	}
	httpresp.OK(c, result)
}

func (s *Server) PostAPIAiSession(c *gin.Context) {
	if s.services.AI == nil {
		httpresp.BadRequest(c, "ai service not configured", "")
		return
	}
	userID, ok := currentUserID(c)
	if !ok {
		httpresp.Unauthorized(c, "missing user")
		return
	}
	req := &service.PostAIChatReq{}
	if c.Request.ContentLength != 0 {
		// 可选请求体：node_id / class_id / system_prompt（用于解析提示词）
		var body struct {
			NodeID       *int64  `json:"node_id"`
			ClassID      *int64  `json:"class_id"`
			SystemPrompt *string `json:"system_prompt"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			httpresp.BadRequest(c, "invalid request body", "")
			return
		}
		if body.NodeID != nil {
			req.NodeID = *body.NodeID
		}
		if body.ClassID != nil {
			req.ClassID = *body.ClassID
		}
		if body.SystemPrompt != nil {
			req.SystemPrompt = *body.SystemPrompt
		}
	}
	sessionID, err := s.services.AI.CreateSession(c.Request.Context(), userID, req)
	if err != nil {
		httpresp.InternalError(c, "create session failed", "")
		return
	}
	httpresp.OK(c, gin.H{"session_id": sessionID})
}

// --- 朋友圈 Moments ---

func (s *Server) GetAPIStuClassClassIDMoments(c *gin.Context, classID int) {
	userID, ok := currentUserID(c)
	if !ok {
		httpresp.Unauthorized(c, "missing user")
		return
	}
	result, err := s.services.Moment.List(c.Request.Context(), int64(classID), userID)
	if err != nil {
		respondError(c, err)
		return
	}
	httpresp.OK(c, result)
}

func (s *Server) PostAPIStuClassClassIDMoments(c *gin.Context, classID int) {
	userID, ok := currentUserID(c)
	if !ok {
		httpresp.Unauthorized(c, "missing user")
		return
	}
	var body api.PostMomentsReq
	if err := c.ShouldBindJSON(&body); err != nil {
		httpresp.BadRequest(c, "invalid request body", err.Error())
		return
	}
	result, err := s.services.Moment.Create(c.Request.Context(), int64(classID), userID, body)
	if err != nil {
		respondError(c, err)
		return
	}
	httpresp.OK(c, result)
}

func (s *Server) DeleteAPIStuClassClassIDMomentsCommentsCommentID(c *gin.Context, classID int, commentID int) {
	userID, ok := currentUserID(c)
	if !ok {
		httpresp.Unauthorized(c, "missing user")
		return
	}
	if err := s.services.Moment.DeleteComment(c.Request.Context(), int64(classID), int64(commentID), userID); err != nil {
		respondError(c, err)
		return
	}
	httpresp.OK(c, gin.H{"ok": true})
}

func (s *Server) GetAPIStuClassClassIDMomentsMeStats(c *gin.Context, classID int) {
	userID, ok := currentUserID(c)
	if !ok {
		httpresp.Unauthorized(c, "missing user")
		return
	}
	result, err := s.services.Moment.GetStats(c.Request.Context(), int64(classID), userID)
	if err != nil {
		respondError(c, err)
		return
	}
	httpresp.OK(c, result)
}

func (s *Server) GetAPIStuClassClassIDMomentsSources(c *gin.Context, classID int) {
	userID, ok := currentUserID(c)
	if !ok {
		httpresp.Unauthorized(c, "missing user")
		return
	}
	result, err := s.services.Moment.GetSources(c.Request.Context(), int64(classID), userID)
	if err != nil {
		respondError(c, err)
		return
	}
	httpresp.OK(c, result)
}

func (s *Server) GetAPIStuClassClassIDMomentsMomentID(c *gin.Context, classID int, momentID int) {
	userID, ok := currentUserID(c)
	if !ok {
		httpresp.Unauthorized(c, "missing user")
		return
	}
	result, err := s.services.Moment.Get(c.Request.Context(), int64(classID), int64(momentID), userID)
	if err != nil {
		respondError(c, err)
		return
	}
	httpresp.OK(c, result)
}

func (s *Server) PostAPIStuClassClassIDMomentsMomentIDComments(c *gin.Context, classID int, momentID int) {
	userID, ok := currentUserID(c)
	if !ok {
		httpresp.Unauthorized(c, "missing user")
		return
	}
	var body api.PostMomentsCommentsReq
	if err := c.ShouldBindJSON(&body); err != nil {
		httpresp.BadRequest(c, "invalid request body", err.Error())
		return
	}
	result, err := s.services.Moment.Comment(c.Request.Context(), int64(classID), int64(momentID), userID, body.Content)
	if err != nil {
		respondError(c, err)
		return
	}
	httpresp.OK(c, result)
}

func (s *Server) DeleteAPIStuClassClassIDMomentsMomentIDLike(c *gin.Context, classID int, momentID int) {
	userID, ok := currentUserID(c)
	if !ok {
		httpresp.Unauthorized(c, "missing user")
		return
	}
	result, err := s.services.Moment.Unlike(c.Request.Context(), int64(classID), int64(momentID), userID)
	if err != nil {
		respondError(c, err)
		return
	}
	httpresp.OK(c, result)
}

func (s *Server) PostAPIStuClassClassIDMomentsMomentIDLike(c *gin.Context, classID int, momentID int) {
	userID, ok := currentUserID(c)
	if !ok {
		httpresp.Unauthorized(c, "missing user")
		return
	}
	result, err := s.services.Moment.Like(c.Request.Context(), int64(classID), int64(momentID), userID)
	if err != nil {
		respondError(c, err)
		return
	}
	httpresp.OK(c, result)
}

// --- 工具通用接口 ---
// UploadFile / DownloadFile / OcrRecognize 实现见 upload.go

// GetTranscription GET /api/utils/transcriptions/{resourceId} 获取语音转写结果（百度短语音识别）。
func (s *Server) GetAPIUtilsTranscriptionsResourceID(c *gin.Context, resourceID string) {
	text, err := s.services.Upload.TranscribeAudio(c.Request.Context(), resourceID)
	if err != nil {
		respondError(c, err)
		return
	}
	httpresp.OK(c, api.UtilsTranscriptionsResp{Text: text})
}

// --- 账号模块 ---

func (s *Server) PostAPIV1AccountsLogin(c *gin.Context) {
	var body api.PostAPIV1AccountsLoginFormdataBody
	if err := c.ShouldBind(&body); err != nil {
		httpresp.BadRequest(c, "invalid form data", err.Error())
		return
	}
	username := ""
	if body.Username != nil {
		username = *body.Username
	}
	password := ""
	if body.Password != nil {
		password = *body.Password
	}
	// 兼容直接 PostForm 调用
	if username == "" {
		username = c.PostForm("username")
	}
	if password == "" {
		password = c.PostForm("password")
	}
	if username == "" || password == "" {
		httpresp.BadRequest(c, "username and password required", "")
		return
	}
	result, err := s.services.Account.Login(c.Request.Context(), username, password)
	if err != nil {
		respondError(c, err)
		return
	}
	httpresp.OK(c, result)
}

func (s *Server) GetAPIV1AccountsStatus(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		httpresp.Unauthorized(c, "missing user")
		return
	}
	status, err := s.services.Account.GetStatus(c.Request.Context(), userID)
	if err != nil {
		respondError(c, err)
		return
	}
	httpresp.OK(c, status)
}

// --- 创作工坊 ---

func (s *Server) PostAPIV1StuClassClassIDNodeNodeIDCreationWorkshopTaskCreationSubmit(c *gin.Context, classID string, nodeID string) {
	httpresp.NotImplemented(c)
}

func (s *Server) PostAPIV1StuClassClassIDNodeNodeIDCreationWorkshopTaskIDExpressSubmit(c *gin.Context, classID int, nodeID int, taskID string) {
	httpresp.NotImplemented(c)
}

func (s *Server) GetAPIV1StuClassClassIDNodeNodeIDCreationWorkshopTaskIDGenerationParams(c *gin.Context, classID string, nodeID string, taskID string) {
	httpresp.NotImplemented(c)
}

func (s *Server) GetAPIV1StuClassClassIDNodeNodeIDCreationWorkshopTaskIDGenerationState(c *gin.Context, classID int, nodeID int, taskID string) {
	httpresp.NotImplemented(c)
}

func (s *Server) PostAPIV1StuClassClassIDNodeNodeIDCreationWorkshopTaskIDGenerationSubmit(c *gin.Context, classID int, nodeID int, taskID string) {
	httpresp.NotImplemented(c)
}

func (s *Server) PostAPIV1StuClassClassIDNodeNodeIDCreationWorkshopTaskIDGenerationSubmitSubmitID(c *gin.Context, classID int, nodeID int, taskID string, submitID string) {
	httpresp.NotImplemented(c)
}

func (s *Server) GetAPIV1StuClassClassIDNodeNodeIDCreationWorkshopTaskIDGenerationSubmitSubmitIDPublishResult(c *gin.Context, classID int, nodeID int, taskID string, submitID string) {
	httpresp.NotImplemented(c)
}

func (s *Server) GetAPIV1StuClassClassIDNodeNodeIDCreationWorkshopTaskIDGenerationSubmitIDResult(c *gin.Context, classID int, nodeID int, taskID string, submitID string) {
	httpresp.NotImplemented(c)
}

func (s *Server) GetAPIV1StuClassClassIDNodeNodeIDCreationWorkshopTaskIDPoemscriptsParams(c *gin.Context, classID int, nodeID int, taskID string) {
	httpresp.NotImplemented(c)
}

func (s *Server) GetAPIV1StuClassClassIDNodeNodeIDCreationWorkshopTaskIDPoemscriptsState(c *gin.Context, classID int, nodeID int, taskID string) {
	httpresp.NotImplemented(c)
}

func (s *Server) PostAPIV1StuClassClassIDNodeNodeIDCreationWorkshopTaskIDPoemscriptsSubmit(c *gin.Context, classID int, nodeID int, taskID string) {
	httpresp.NotImplemented(c)
}

func (s *Server) PostAPIV1StuClassClassIDNodeNodeIDCreationWorkshopTaskIDPoemscriptsSubmitSubmitID(c *gin.Context, classID int, nodeID int, taskID string, submitID string) {
	httpresp.NotImplemented(c)
}

func (s *Server) GetAPIV1StuClassClassIDNodeNodeIDCreationWorkshopTaskIDPoemscriptsSubmitSubmitIDPublishResult(c *gin.Context, classID int, nodeID int, taskID string, submitID string) {
	httpresp.NotImplemented(c)
}

func (s *Server) GetAPIV1StuClassClassIDNodeNodeIDCreationWorkshopTaskIDPoemscriptsSubmitIDResult(c *gin.Context, classID int, nodeID int, taskID string, submitID string) {
	httpresp.NotImplemented(c)
}

func (s *Server) PostAPIV1StuClassClassIDNodeNodeIDCreationWorkshopTaskIDShareSubmit(c *gin.Context, classID int, nodeID int, taskID string) {
	httpresp.NotImplemented(c)
}

func (s *Server) GetAPIV1StuClassClassIDNodeNodeIDCreationWorkshopTaskIDShareSubmitIDResult(c *gin.Context, classID int, nodeID int, taskID string, submitID string) {
	httpresp.NotImplemented(c)
}

func (s *Server) GetAPIV1StuClassClassIDNodeNodeIDCreationWorkshopTaskIDTextcontent(c *gin.Context, classID int, nodeID int, taskID int) {
	httpresp.NotImplemented(c)
}

// --- 文学风格 cultural-style ---

func (s *Server) GetAPIV1StuClassClassIDNodeNodeIDCulturalStyleBreakpointsDragSortBpIDParams(c *gin.Context, classID int, nodeID int, bpid string) {
	params, err := s.services.CulturalStyle.GetDragSortParams(c.Request.Context(), int64(nodeID), bpid)
	if err != nil {
		respondError(c, err)
		return
	}
	httpresp.OK(c, params)
}

func (s *Server) GetAPIV1StuClassClassIDNodeNodeIDCulturalStyleBreakpointsDragSortBpIDState(c *gin.Context, classID int, nodeID int, bpid string) {
	userID, ok := currentUserID(c)
	if !ok {
		httpresp.Unauthorized(c, "missing user")
		return
	}
	state, err := s.services.CulturalStyle.GetDragSortState(c.Request.Context(), int64(classID), int64(nodeID), userID, bpid)
	if err != nil {
		respondError(c, err)
		return
	}
	httpresp.OK(c, state)
}

func (s *Server) PostAPIV1StuClassClassIDNodeNodeIDCulturalStyleBreakpointsDragSortBpIDSubmissions(c *gin.Context, classID int, nodeID int, bpid string) {
	userID, ok := currentUserID(c)
	if !ok {
		httpresp.Unauthorized(c, "missing user")
		return
	}
	var body api.PostAPIV1StuClassClassIDNodeNodeIDCulturalStyleBreakpointsDragSortBpIDSubmissionsJSONRequestBody
	if err := c.ShouldBindJSON(&body); err != nil {
		httpresp.BadRequest(c, "invalid request body", err.Error())
		return
	}
	result, err := s.services.CulturalStyle.SubmitDragSort(c.Request.Context(), int64(classID), int64(nodeID), userID, bpid, body.Answer)
	if err != nil {
		respondError(c, err)
		return
	}
	httpresp.OK(c, result)
}

func (s *Server) GetAPIV1StuClassClassIDNodeNodeIDCulturalStyleBreakpointsDragSortBpIDSubmissionsSubmitID(c *gin.Context, classID int, nodeID int, bpid string, submitID string) {
	userID, ok := currentUserID(c)
	if !ok {
		httpresp.Unauthorized(c, "missing user")
		return
	}
	result, err := s.services.CulturalStyle.GetDragSortSubmission(c.Request.Context(), int64(nodeID), userID, submitID)
	if err != nil {
		respondError(c, err)
		return
	}
	httpresp.OK(c, result)
}

func (s *Server) GetAPIV1StuClassClassIDNodeNodeIDCulturalStyleBreakpointsQuickSelectBpIDParams(c *gin.Context, classID int, nodeID int, bpid string) {
	params, err := s.services.CulturalStyle.GetQuickSelectParams(c.Request.Context(), int64(nodeID), bpid)
	if err != nil {
		respondError(c, err)
		return
	}
	httpresp.OK(c, params)
}

func (s *Server) PostAPIV1StuClassClassIDNodesNodeIDCulturalStyleBreakpointsQuickSelectBpIDSubmit(c *gin.Context, classID int, nodeID int, bpid string) {
	userID, ok := currentUserID(c)
	if !ok {
		httpresp.Unauthorized(c, "missing user")
		return
	}
	var body api.PostAPIV1StuClassClassIDNodesNodeIDCulturalStyleBreakpointsQuickSelectBpIDSubmitJSONRequestBody
	if err := c.ShouldBindJSON(&body); err != nil {
		httpresp.BadRequest(c, "invalid request body", err.Error())
		return
	}
	result, err := s.services.CulturalStyle.SubmitQuickSelect(c.Request.Context(), int64(classID), int64(nodeID), userID, bpid, body.SelectedID)
	if err != nil {
		respondError(c, err)
		return
	}
	httpresp.OK(c, result)
}

func (s *Server) GetAPIV1StuClassClassIDNodeNodeIDCulturalStyleEnding(c *gin.Context, classID int, nodeID int) {
	userID, ok := currentUserID(c)
	if !ok {
		httpresp.Unauthorized(c, "missing user")
		return
	}
	result, err := s.services.CulturalStyle.GetEnding(c.Request.Context(), int64(classID), int64(nodeID), userID)
	if err != nil {
		respondError(c, err)
		return
	}
	httpresp.OK(c, result)
}

func (s *Server) GetAPIV1StuClassClassIDNodeNodeIDCulturalStyleParams(c *gin.Context, classID int, nodeID int) {
	userID, ok := currentUserID(c)
	if !ok {
		httpresp.Unauthorized(c, "missing user")
		return
	}
	params, err := s.services.CulturalStyle.GetParams(c.Request.Context(), int64(classID), int64(nodeID), userID)
	if err != nil {
		respondError(c, err)
		return
	}
	httpresp.OK(c, params)
}

func (s *Server) PatchAPIV1StuClassClassIDNodeNodeIDCuturalStyleStateSyncTime(c *gin.Context, classID int, nodeID int) {
	userID, ok := currentUserID(c)
	if !ok {
		httpresp.Unauthorized(c, "missing user")
		return
	}
	var body api.PatchAPIV1StuClassClassIDNodeNodeIDCuturalStyleStateSyncTimeJSONRequestBody
	if err := c.ShouldBindJSON(&body); err != nil {
		httpresp.BadRequest(c, "invalid request body", err.Error())
		return
	}
	if err := s.services.CulturalStyle.SyncTime(c.Request.Context(), int64(classID), int64(nodeID), userID, body.CurrentTime); err != nil {
		respondError(c, err)
		return
	}
	httpresp.OK(c, map[string]string{"status": "ok"})
}

func (s *Server) GetAPIV1StuClassClassIDNodeNodeIDCulturalStyleState(c *gin.Context, classID int, nodeID int) {
	userID, ok := currentUserID(c)
	if !ok {
		httpresp.Unauthorized(c, "missing user")
		return
	}
	state, err := s.services.CulturalStyle.GetState(c.Request.Context(), int64(classID), int64(nodeID), userID)
	if err != nil {
		respondError(c, err)
		return
	}
	httpresp.OK(c, state)
}

// --- heritage-cultural（实现见 heritage_cultural.go）---
func (s *Server) GetAPIV1StuClassClassIDNodeNodeIDHeritageCulturalParams(c *gin.Context, classID int, nodeID int) {
	params, err := s.services.HeritageCultural.GetParams(c.Request.Context(), int64(nodeID))
	if err != nil {
		respondError(c, err)
		return
	}
	httpresp.OK(c, params)
}

// GetAPIV1StuClassClassIDNodeNodeIDHeritageCulturalState GET .../heritage-cultural/state
// 返回该节点的历史提交记录。
func (s *Server) GetAPIV1StuClassClassIDNodeNodeIDHeritageCulturalState(c *gin.Context, classID int, nodeID int) {
	userID, ok := currentUserID(c)
	if !ok {
		httpresp.Unauthorized(c, "missing user")
		return
	}
	state, err := s.services.HeritageCultural.GetState(c.Request.Context(), int64(classID), int64(nodeID), userID)
	if err != nil {
		respondError(c, err)
		return
	}
	httpresp.OK(c, state)
}

// PostAPIV1StuClassClassIDNodeNodeIDHeritageCulturalSubmissions POST .../heritage-cultural/submissions
// 学生提交一段讲解文本，异步触发 AI 评测，返回 submitId 供前端轮询。
func (s *Server) PostAPIV1StuClassClassIDNodeNodeIDHeritageCulturalSubmissions(c *gin.Context, classID int, nodeID int) {
	userID, ok := currentUserID(c)
	if !ok {
		httpresp.Unauthorized(c, "missing user")
		return
	}
	var body api.PostAPIV1StuClassClassIDNodeNodeIDHeritageCulturalSubmissionsJSONRequestBody
	if err := c.ShouldBindJSON(&body); err != nil || body.Text == "" {
		httpresp.BadRequest(c, "text 不能为空", "")
		return
	}
	resp, err := s.services.HeritageCultural.Submit(c.Request.Context(), int64(classID), int64(nodeID), userID, body.Text)
	if err != nil {
		respondError(c, err)
		return
	}
	httpresp.OK(c, resp)
}

// GetAPIV1StuClassClassIDNodeNodeIDHeritageCulturalSubmissionsSubmitID GET .../heritage-cultural/submissions/{submitId}
// 前端拿 submitId 轮询本次评测结果。
func (s *Server) GetAPIV1StuClassClassIDNodeNodeIDHeritageCulturalSubmissionsSubmitID(c *gin.Context, classID int, nodeID int, submitID string) {
	userID, ok := currentUserID(c)
	if !ok {
		httpresp.Unauthorized(c, "missing user")
		return
	}
	result, err := s.services.HeritageCultural.GetSubmission(c.Request.Context(), int64(nodeID), userID, submitID)
	if err != nil {
		respondError(c, err)
		return
	}
	httpresp.OK(c, result)
}

// GetAPIV4StuClassClassIDNodeNodeIDHeritageCulturalEnding GET /api/v4/.../heritage-cultural/ending
// 结束任务，返回总评信息。
func (s *Server) GetAPIV4StuClassClassIDNodeNodeIDHeritageCulturalEnding(c *gin.Context, classID int, nodeID int) {
	userID, ok := currentUserID(c)
	if !ok {
		httpresp.Unauthorized(c, "missing user")
		return
	}
	result, err := s.services.HeritageCultural.GetEnding(c.Request.Context(), int64(classID), int64(nodeID), userID)
	if err != nil {
		respondError(c, err)
		return
	}
	httpresp.OK(c, result)
}

// --- initial-insight ---

func (s *Server) GetAPIV1StuClassClassIDNodeNodeIDInitialInsightState(c *gin.Context, classID int, nodeID int) {
	userID, ok := currentUserID(c)
	if !ok {
		httpresp.Unauthorized(c, "missing user")
		return
	}
	state, err := s.services.InitialInsight.GetState(c.Request.Context(), int64(classID), int64(nodeID), userID)
	if err != nil {
		respondError(c, err)
		return
	}
	httpresp.OK(c, state)
}

func (s *Server) PostAPIV1StuClassClassIDNodeNodeIDInitialInsightQuestionIDSubmissions(c *gin.Context, classID int, nodeID int, questionID string) {
	userID, ok := currentUserID(c)
	if !ok {
		httpresp.Unauthorized(c, "missing user")
		return
	}
	var body api.PostAPIV1StuClassClassIDNodeNodeIDInitialInsightQuestionIDSubmissionsJSONRequestBody
	if err := c.ShouldBindJSON(&body); err != nil {
		httpresp.BadRequest(c, "invalid request body", err.Error())
		return
	}
	req := service.InitialInsightSubmitRequest{
		QuestionID: body.QuestionID,
		BlankID:    body.BlankID,
		Input:      body.Input,
	}
	result, err := s.services.InitialInsight.Submit(c.Request.Context(), int64(classID), int64(nodeID), userID, req)
	if err != nil {
		respondError(c, err)
		return
	}
	// submit 接口契约返回 AsyncSubmitResp（submitId + tip），异步批改后前端轮询 GetInitialInsightSubmission
	httpresp.OK(c, result)
}

func (s *Server) GetAPIV1StuClassClassIDNodeNodeIDInitialInsightParams(c *gin.Context, classID int, nodeID int) {
	userID, ok := currentUserID(c)
	if !ok {
		httpresp.Unauthorized(c, "missing user")
		return
	}
	params, err := s.services.InitialInsight.GetParams(c.Request.Context(), int64(classID), int64(nodeID), userID)
	if err != nil {
		respondError(c, err)
		return
	}
	httpresp.OK(c, params)
}

func (s *Server) GetAPIV1StuClassClassIDNodeNodeIDInitialInsightQuestionIDSubmissionsSubmitID(c *gin.Context, classID int, nodeID int, questionID string, submitID string) {
	userID, ok := currentUserID(c)
	if !ok {
		httpresp.Unauthorized(c, "missing user")
		return
	}
	result, err := s.services.InitialInsight.GetSubmission(c.Request.Context(), int64(nodeID), userID, submitID)
	if err != nil {
		respondError(c, err)
		return
	}
	httpresp.OK(c, result)
}

func (s *Server) GetAPIV4StuClassClassIDNodeNodeIDInitialInsightEnding(c *gin.Context, classID int, nodeID int) {
	userID, ok := currentUserID(c)
	if !ok {
		httpresp.Unauthorized(c, "missing user")
		return
	}
	result, err := s.services.InitialInsight.GetEnding(c.Request.Context(), int64(classID), int64(nodeID), userID)
	if err != nil {
		respondError(c, err)
		return
	}
	httpresp.OK(c, result)
}

// --- write-thoughts ---

func (s *Server) GetAPIV1StuClassClassIDNodeNodeIDWriteThoughtsParams(c *gin.Context, classID int, nodeID int) {
	userID, ok := currentUserID(c)
	if !ok {
		httpresp.Unauthorized(c, "missing user")
		return
	}
	params, err := s.services.WriteThoughts.GetParams(c.Request.Context(), int64(classID), int64(nodeID), userID)
	if err != nil {
		respondError(c, err)
		return
	}
	httpresp.OK(c, params)
}

func (s *Server) GetAPIV1StuClassClassIDNodeNodeIDWriteThoughtsState(c *gin.Context, classID int, nodeID int) {
	userID, ok := currentUserID(c)
	if !ok {
		httpresp.Unauthorized(c, "missing user")
		return
	}
	state, err := s.services.WriteThoughts.GetState(c.Request.Context(), int64(classID), int64(nodeID), userID)
	if err != nil {
		respondError(c, err)
		return
	}
	httpresp.OK(c, state)
}

func (s *Server) PostAPIV1StuClassClassIDNodeNodeIDWriteThoughtsSubmissions(c *gin.Context, classID int, nodeID int) {
	userID, ok := currentUserID(c)
	if !ok {
		httpresp.Unauthorized(c, "missing user")
		return
	}
	var body api.PostAPIV1StuClassClassIDNodeNodeIDWriteThoughtsSubmissionsJSONRequestBody
	if err := c.ShouldBindJSON(&body); err != nil {
		httpresp.BadRequest(c, "invalid request body", err.Error())
		return
	}
	req := service.SubmitRequest{
		QuestionID: body.QuestionID,
		Input:      body.Input,
		Type:       string(body.Type),
	}
	result, err := s.services.WriteThoughts.Submit(c.Request.Context(), int64(classID), int64(nodeID), userID, req)
	if err != nil {
		respondError(c, err)
		return
	}
	// submit 接口契约返回 AsyncSubmitResp（submitId + tip），异步评分后前端轮询 GetSubmission
	httpresp.OK(c, result)
}

func (s *Server) GetAPIV1StuClassClassIDNodeNodeIDWriteThoughtsSubmissionsSubmitID(c *gin.Context, classID int, nodeID int, submitID string) {
	userID, ok := currentUserID(c)
	if !ok {
		httpresp.Unauthorized(c, "missing user")
		return
	}
	result, err := s.services.WriteThoughts.GetSubmission(c.Request.Context(), int64(nodeID), userID, submitID)
	if err != nil {
		respondError(c, err)
		return
	}
	httpresp.OK(c, result)
}

// --- wenmingzhongwai ---

func (s *Server) GetAPIV1StuClassClassIDNodeNodeIDWenmingzhongwaiParams(c *gin.Context, classID int, nodeID int) {
	params, err := s.services.NodeState.GetWenMingZhongWaiParams(c.Request.Context(), int64(nodeID))
	if err != nil {
		respondError(c, err)
		return
	}
	httpresp.OK(c, params)
}

func (s *Server) GetAPIV1StuClassClassIDNodeNodeIDWenmingzhongwaiState(c *gin.Context, classID int, nodeID int) {
	userID, ok := currentUserID(c)
	if !ok {
		httpresp.Unauthorized(c, "missing user")
		return
	}
	state, err := s.services.NodeState.GetWenMingZhongWaiState(c.Request.Context(), int64(classID), int64(nodeID), userID)
	if err != nil {
		respondError(c, err)
		return
	}
	httpresp.OK(c, state)
}

func (s *Server) GetAPIV1StuClassClassIDNodeNodeIDWenmingzhongwaiSubmissionsSubmitID(c *gin.Context, classID int, nodeID int, submitID string) {
	userID, ok := currentUserID(c)
	if !ok {
		httpresp.Unauthorized(c, "missing user")
		return
	}
	result, err := s.services.WenMingZhongWai.GetSubmission(c.Request.Context(), int64(nodeID), userID, submitID)
	if err != nil {
		respondError(c, err)
		return
	}
	httpresp.OK(c, result)
}

func (s *Server) PostAPIV1StuClassClassIDNodeNodeIDWenmingzhongwaiQuestionIDSubmissions(c *gin.Context, classID int, nodeID int, questionID string) {
	userID, ok := currentUserID(c)
	if !ok {
		httpresp.Unauthorized(c, "missing user")
		return
	}
	var body api.PostAPIV1StuClassClassIDNodeNodeIDWenmingzhongwaiQuestionIDSubmissionsJSONRequestBody
	if err := c.ShouldBindJSON(&body); err != nil {
		httpresp.BadRequest(c, "invalid request body", err.Error())
		return
	}
	answers := map[string]string{}
	if body.Answers != nil {
		answers = *body.Answers
	}
	req := service.WenMingZhongWaiSubmitRequest{
		Answers: answers,
		Type:    string(body.Type),
	}
	resp, err := s.services.WenMingZhongWai.Submit(c.Request.Context(), int64(classID), int64(nodeID), userID, questionID, req)
	if err != nil {
		respondError(c, err)
		return
	}
	httpresp.OK(c, resp)
}

// --- zhaozhouqiao ---

func (s *Server) GetAPIV1StuClassClassIDNodeNodeIDZhaozhouqiaoParams(c *gin.Context, classID int, nodeID int) {
	params, err := s.services.NodeState.GetZhaoZhouQiaoParams(c.Request.Context(), int64(nodeID))
	if err != nil {
		respondError(c, err)
		return
	}
	httpresp.OK(c, params)
}

func (s *Server) GetAPIV1StuClassClassIDNodeNodeIDZhaozhouqiaoState(c *gin.Context, classID int, nodeID int) {
	userID, ok := currentUserID(c)
	if !ok {
		httpresp.Unauthorized(c, "missing user")
		return
	}
	state, err := s.services.NodeState.GetZhaoZhouQiaoState(c.Request.Context(), int64(classID), int64(nodeID), userID)
	if err != nil {
		respondError(c, err)
		return
	}
	httpresp.OK(c, state)
}

func (s *Server) GetAPIV1StuClassClassIDNodeNodeIDZhaozhouqiaoSubmissionsSubmitID(c *gin.Context, classID int, nodeID int, submitID string) {
	userID, ok := currentUserID(c)
	if !ok {
		httpresp.Unauthorized(c, "missing user")
		return
	}
	result, err := s.services.ZhaoZhouQiao.GetSubmission(c.Request.Context(), int64(nodeID), userID, submitID)
	if err != nil {
		respondError(c, err)
		return
	}
	httpresp.OK(c, result)
}

func (s *Server) PostAPIV1StuClassClassIDNodeNodeIDZhaozhouqiaoQuestionIDSubmissions(c *gin.Context, classID int, nodeID int, questionID string) {
	userID, ok := currentUserID(c)
	if !ok {
		httpresp.Unauthorized(c, "missing user")
		return
	}
	var body api.PostAPIV1StuClassClassIDNodeNodeIDZhaozhouqiaoQuestionIDSubmissionsJSONRequestBody
	if err := c.ShouldBindJSON(&body); err != nil {
		httpresp.BadRequest(c, "invalid request body", err.Error())
		return
	}
	req := service.ZhaoZhouQiaoSubmitRequest{
		CardID: body.CardID,
		Type:   string(body.Type),
	}
	if body.Text != nil {
		req.Text = *body.Text
	}
	if body.Audio != nil {
		req.AudioID = body.Audio.ID
	}
	resp, err := s.services.ZhaoZhouQiao.Submit(c.Request.Context(), int64(classID), int64(nodeID), userID, questionID, req)
	if err != nil {
		respondError(c, err)
		return
	}
	httpresp.OK(c, resp)
}

// --- 报告与事件 ---

func (s *Server) GetAPIV1StuClassClassIDNodeNodeIDEvaluation(c *gin.Context, classID int, nodeID int) {
	userID, ok := currentUserID(c)
	if !ok {
		httpresp.Unauthorized(c, "missing user")
		return
	}
	result, err := s.services.Report.GetNodeEvaluation(c.Request.Context(), int64(classID), int64(nodeID), userID)
	if err != nil {
		respondError(c, err)
		return
	}
	httpresp.OK(c, result)
}

func (s *Server) GetAPIV1StuClassClassIDNodeNodeIDReport(c *gin.Context, classID int, nodeID int) {
	userID, ok := currentUserID(c)
	if !ok {
		httpresp.Unauthorized(c, "missing user")
		return
	}
	result, err := s.services.Report.GetStuNodeReport(c.Request.Context(), int64(classID), int64(nodeID), userID)
	if err != nil {
		respondError(c, err)
		return
	}
	httpresp.OK(c, result)
}

// GetAPIV1StuClassClassID 获取课堂详细信息。
func (s *Server) GetAPIV1StuClassClassID(c *gin.Context, classID int) {
	userID, ok := currentUserID(c)
	if !ok {
		httpresp.Unauthorized(c, "missing user")
		return
	}
	detail, err := s.services.NodeState.GetClassDetail(c.Request.Context(), int64(classID), userID)
	if err != nil {
		respondError(c, err)
		return
	}
	httpresp.OK(c, detail)
}

func (s *Server) GetAPIV1StuClassClassIDReport(c *gin.Context, classID int) {
	userID, ok := currentUserID(c)
	if !ok {
		httpresp.Unauthorized(c, "missing user")
		return
	}
	result, err := s.services.Report.GetStuClassReport(c.Request.Context(), int64(classID), userID)
	if err != nil {
		respondError(c, err)
		return
	}
	httpresp.OK(c, result)
}

func (s *Server) PostAPIV1StuEventsEnterClassNode(c *gin.Context) {
	httpresp.NotImplemented(c)
}
