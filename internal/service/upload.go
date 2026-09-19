package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"zhonghuawenhua_backend/internal/api"
	"zhonghuawenhua_backend/internal/model"
	"zhonghuawenhua_backend/pkg/ocr"
	"zhonghuawenhua_backend/pkg/stt"
	"zhonghuawenhua_backend/pkg/tts"
)

// UploadService 文件上传下载、OCR 识别、语音转写与合成业务。
type UploadService struct {
	repos      *Repositories
	uploadDir  string // 文件保存目录（相对工作目录）
	storageURL string // 上传资源对外访问 URL 前缀（如 /storage/）
	ocr        *ocr.Baidu
	stt        *stt.Baidu
	tts        *tts.Baidu
}

// 文件大小上限（防御性要求：防写满磁盘 / 防全量读入内存）。
const (
	// maxUploadFileSize 单文件上传大小上限（50MB），与 handler 侧 maxUploadSize 一致。
	maxUploadFileSize = 50 << 20
	// maxOCRFileSize 识别图片大小上限（10MB；百度 OCR base64 上限约 4MB，留余量）。
	maxOCRFileSize = 10 << 20
	// maxSTTFileSize 转写音频大小上限（10MB；百度短语音识别 base64 后上限约 4MB，留余量）。
	maxSTTFileSize = 10 << 20
)

// NewUploadService 创建 UploadService，uploadDir 为空时默认 "upload"；storageURL 为空时默认 "/storage/"。
func NewUploadService(repos *Repositories, uploadDir, storageURL string, ocrClient *ocr.Baidu, sttClient *stt.Baidu, ttsClient *tts.Baidu) *UploadService {
	if uploadDir == "" {
		uploadDir = "upload"
	}
	if storageURL == "" {
		storageURL = "/storage/"
	}
	storageURL = "/" + strings.Trim(storageURL, "/") + "/"
	return &UploadService{repos: repos, uploadDir: uploadDir, storageURL: storageURL, ocr: ocrClient, stt: sttClient, tts: ttsClient}
}

// UploadReq 上传请求参数（来自 multipart 表单）。
type UploadReq struct {
	MediaType      string // 文件类型：image / audio / video
	ResourceSource string // 文件来源
	ResourceInfo   string // 文件简介
	ImageCompress  bool
	VideoCompress  bool
	UploadOSS      bool // 是否上传保存
	OwnerID        int64
}

// Upload 保存上传文件到 uploadDir，并登记 resources 记录，返回上传响应（id 为数据库自增 id）。
func (s *UploadService) Upload(ctx context.Context, file multipart.File, header *multipart.FileHeader, req UploadReq) (*api.UtilsUploadResp, error) {
	mediaType := model.ResourceMediaType(req.MediaType)
	if !mediaType.Valid() {
		return nil, ErrInvalidMediaType
	}
	// 防御性要求：文件大小上限校验（防超大文件写满磁盘）
	if header.Size > maxUploadFileSize {
		return nil, ErrFileTooLarge
	}

	// 唯一资源 id 作为文件名，避免重名覆盖
	resourceID := fmt.Sprintf("rs%d", time.Now().UnixNano())
	ext := strings.ToLower(filepath.Ext(header.Filename))
	relPath := resourceID + ext
	savePath := filepath.Join(s.uploadDir, relPath)
	// 相对后端 baseUrl 的资源访问路径（Nginx 等静态服务映射 storageURL 到 uploadDir）
	fileURL := s.storageURL + relPath

	if err := os.MkdirAll(s.uploadDir, 0o755); err != nil {
		return nil, fmt.Errorf("create upload dir: %w", err)
	}
	dst, err := os.Create(savePath)
	if err != nil {
		return nil, fmt.Errorf("create upload file: %w", err)
	}
	defer dst.Close()
	if _, err := io.Copy(dst, file); err != nil {
		return nil, fmt.Errorf("save upload file: %w", err)
	}

	var ownerID *int64
	if req.OwnerID > 0 {
		ownerID = &req.OwnerID
	}
	res := &model.Resource{
		ResourceID:     resourceID,
		MediaType:      mediaType,
		FileName:       header.Filename,
		FileURL:        &fileURL, // 相对 baseUrl 的资源访问路径
		ResourceSuffix: strings.TrimPrefix(ext, "."),
		ResourceSize:   header.Size,
		ResourceInfo:   strPtrIfSet(req.ResourceInfo),
		ResourceSource: strPtrIfSet(req.ResourceSource),
		UploadOSS:      req.UploadOSS,
		ImageCompress:  req.ImageCompress,
		VideoCompress:  req.VideoCompress,
		OwnerID:        ownerID,
	}

	id, err := s.repos.Resource.Create(ctx, res)
	if err != nil {
		return nil, err
	}
	res.ID = id
	resp := res.ToUploadResp()
	return &resp, nil
}

// DownloadURL 按资源 id（数字数据库 id 或唯一 resource_id）查询文件，返回其相对访问 url（前端拼接后端 baseUrl）。
func (s *UploadService) DownloadURL(ctx context.Context, id string) (*api.UtilsDownloadResp, error) {
	res, err := s.resolveResource(ctx, id)
	if err != nil {
		return nil, err
	}
	if res.FileURL == nil {
		return nil, fmt.Errorf("resource file url is empty")
	}
	return &api.UtilsDownloadResp{URL: *res.FileURL}, nil
}

// RecognizeOCR 按 resourceId（数字数据库 id 或唯一 resource_id）解析本地图片资源并识别文字。
func (s *UploadService) RecognizeOCR(ctx context.Context, resourceID string) (string, error) {
	if s.ocr == nil {
		return "", ErrOCRNotConfigured
	}
	res, err := s.resolveResource(ctx, resourceID)
	if err != nil {
		return "", err
	}
	if res.MediaType != model.ResourceMediaTypeImage {
		return "", ErrNotImage
	}
	if res.FileURL == nil {
		return "", fmt.Errorf("resource file url is empty")
	}
	// 防御性要求：读取前先检查文件大小，避免超大图片全量读入内存
	info, err := os.Stat(s.resolveUploadFilePath(*res.FileURL))
	if err != nil {
		return "", fmt.Errorf("stat image file: %w", err)
	}
	if info.Size() > maxOCRFileSize {
		return "", ErrFileTooLarge
	}
	image, err := os.ReadFile(s.resolveUploadFilePath(*res.FileURL))
	if err != nil {
		return "", fmt.Errorf("read image file: %w", err)
	}
	return s.ocr.Recognize(ctx, image)
}

// TranscribeAudio 按 resourceId（数字数据库 id 或唯一 resource_id）解析本地音频资源并调用百度语音识别转写为文本。
// 音频格式由资源后缀确定，仅支持百度短语音识别支持的格式（pcm/wav/amr/m4a/mp3/aac）。
func (s *UploadService) TranscribeAudio(ctx context.Context, resourceID string) (string, error) {
	if s.stt == nil {
		return "", ErrSTTNotConfigured
	}
	res, err := s.resolveResource(ctx, resourceID)
	if err != nil {
		return "", err
	}
	if res.MediaType != model.ResourceMediaTypeAudio {
		return "", ErrNotAudio
	}
	if res.FileURL == nil {
		return "", fmt.Errorf("resource file url is empty")
	}
	format, ok := stt.SupportedFormats[strings.ToLower(res.ResourceSuffix)]
	if !ok {
		return "", ErrUnsupportedAudioFormat
	}
	// 防御性要求：读取前先检查文件大小，避免超大音频全量读入内存
	info, err := os.Stat(s.resolveUploadFilePath(*res.FileURL))
	if err != nil {
		return "", fmt.Errorf("stat audio file: %w", err)
	}
	if info.Size() > maxSTTFileSize {
		return "", ErrFileTooLarge
	}
	audio, err := os.ReadFile(s.resolveUploadFilePath(*res.FileURL))
	if err != nil {
		return "", fmt.Errorf("read audio file: %w", err)
	}
	return s.stt.Recognize(ctx, format, audio)
}

// SynthesizeSpeech 将文本合成为语音（百度 TTS），保存为 mp3 资源并返回相对访问 URL。
func (s *UploadService) SynthesizeSpeech(ctx context.Context, text string) (*api.UtilsDownloadResp, error) {
	if s.tts == nil {
		return nil, ErrTTSNotConfigured
	}
	audio, err := s.tts.Synthesize(ctx, text)
	if err != nil {
		return nil, err
	}
	// 唯一资源 id 作为文件名，避免重名覆盖
	resourceID := fmt.Sprintf("tts%d", time.Now().UnixNano())
	relPath := resourceID + ".mp3"
	savePath := filepath.Join(s.uploadDir, relPath)
	// 相对后端 baseUrl 的资源访问路径
	fileURL := s.storageURL + relPath
	if err := os.MkdirAll(s.uploadDir, 0o755); err != nil {
		return nil, fmt.Errorf("create upload dir: %w", err)
	}
	if err := os.WriteFile(savePath, audio, 0o644); err != nil {
		return nil, fmt.Errorf("save tts audio: %w", err)
	}
	res := &model.Resource{
		ResourceID:     resourceID,
		MediaType:      model.ResourceMediaTypeAudio,
		FileName:       relPath,
		FileURL:        &fileURL,
		ResourceSuffix: "mp3",
		ResourceSize:   int64(len(audio)),
		ResourceInfo:   strPtrIfSet(text),
		ResourceSource: strPtrIfSet("tts"),
	}
	if _, err := s.repos.Resource.Create(ctx, res); err != nil {
		return nil, err
	}
	return &api.UtilsDownloadResp{URL: fileURL}, nil
}

// resolveUploadFilePath 将 resources.file_url 解析为 uploadDir 下的磁盘路径。
// file_url 为 URL 路径（如 "/upload/rs...mp4"），兼容历史裸文件名数据（如 "rs...mp4"）。
func (s *UploadService) resolveUploadFilePath(fileURL string) string {
	name := fileURL[strings.LastIndex(fileURL, "/")+1:]
	return filepath.Join(s.uploadDir, name)
}

// resolveResource 兼容数字数据库 id 与唯一 resource_id 两种方式定位资源。
func (s *UploadService) resolveResource(ctx context.Context, idOrResourceID string) (*model.Resource, error) {
	var (
		res *model.Resource
		err error
	)
	if id, perr := strconv.ParseInt(idOrResourceID, 10, 64); perr == nil {
		res, err = s.repos.Resource.GetByID(ctx, id)
	} else {
		res, err = s.repos.Resource.GetByResourceID(ctx, idOrResourceID)
	}
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrResourceNotFound
		}
		return nil, err
	}
	return res, nil
}
