package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"zhonghuawenhua_backend/internal/api"
	"zhonghuawenhua_backend/internal/service"
	"zhonghuawenhua_backend/pkg/httpresp"
)

// maxUploadSize 上传请求体大小上限（50MB），与 service 侧 maxUploadFileSize 一致。
const maxUploadSize = 50 << 20

// PostAPIUtilsUpload POST /api/utils/upload 上传单个文件，返回资源 id（数据库自增 id）。
func (s *Server) PostAPIUtilsUpload(c *gin.Context) {
	userID, _ := currentUserID(c)

	// 防御性要求：限制请求体大小，防止超大文件写满磁盘（MAX_UPLOAD_SIZE=50MB）
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxUploadSize)

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		httpresp.BadRequest(c, "file 不能为空", "")
		return
	}
	defer file.Close()

	req := service.UploadReq{
		MediaType:      c.PostForm("media_type"),
		ResourceSource: c.PostForm("resource_source"),
		ResourceInfo:   c.PostForm("resource_info"),
		ImageCompress:  c.PostForm("image_compress") == "true",
		VideoCompress:  c.PostForm("video_compress") == "true",
		UploadOSS:      c.PostForm("upload_oss") != "false",
		OwnerID:        userID,
	}

	resp, err := s.services.Upload.Upload(c.Request.Context(), file, header, req)
	if err != nil {
		respondError(c, err)
		return
	}
	httpresp.OK(c, resp)
}

// GetAPIUtilsDownloadID GET /api/utils/download/{id} 按资源 id（数字数据库 id 或唯一 resource_id）返回文件的相对访问 url。
func (s *Server) GetAPIUtilsDownloadID(c *gin.Context, id string, params api.GetAPIUtilsDownloadIDParams) {
	resp, err := s.services.Upload.DownloadURL(c.Request.Context(), id)
	if err != nil {
		respondError(c, err)
		return
	}
	httpresp.OK(c, resp)
}

// PostAPIUtilsOcr POST /api/utils/ocr 识别图片资源中的文字（百度智能云 OCR）。
func (s *Server) PostAPIUtilsOcr(c *gin.Context) {
	var body api.PostAPIUtilsOcrJSONRequestBody
	if err := c.ShouldBindJSON(&body); err != nil || body.ResourceID == "" {
		httpresp.BadRequest(c, "resourceId 不能为空", "")
		return
	}
	text, err := s.services.Upload.RecognizeOCR(c.Request.Context(), body.ResourceID)
	if err != nil {
		respondError(c, err)
		return
	}
	httpresp.OK(c, gin.H{"text": text})
}

// PostAPIUtilsTts POST /api/utils/tts 将文本合成为语音（百度智能云 TTS），返回音频相对 URL。
func (s *Server) PostAPIUtilsTts(c *gin.Context, params api.PostAPIUtilsTtsParams) {
	if strings.TrimSpace(params.Text) == "" {
		httpresp.BadRequest(c, "text 不能为空", "")
		return
	}
	resp, err := s.services.Upload.SynthesizeSpeech(c.Request.Context(), params.Text)
	if err != nil {
		respondError(c, err)
		return
	}
	httpresp.OK(c, resp)
}
