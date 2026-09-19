package model

import (
	"time"

	"zhonghuawenhua_backend/internal/api"
)

// Resource 文件资源表行（对应 resources 表）。
type Resource struct {
	ID              int64            `db:"id" json:"id"`
	ResourceID      string           `db:"resource_id" json:"resourceId"`
	MediaType       ResourceMediaType `db:"media_type" json:"mediaType"`
	FileName        string           `db:"file_name" json:"fileName"`
	FileURL         *string          `db:"file_url" json:"fileUrl,omitempty"`
	ResourceSuffix  string           `db:"resource_suffix" json:"resourceSuffix"`
	ResourceSize    int64            `db:"resource_size" json:"resourceSize"`
	ResourceInfo    *string          `db:"resource_info" json:"resourceInfo,omitempty"`
	ResourceSource  *string          `db:"resource_source" json:"resourceSource,omitempty"`
	UploadOSS       bool             `db:"upload_oss" json:"uploadOss"`
	ImageCompress   bool             `db:"image_compress" json:"imageCompress"`
	VideoCompress   bool             `db:"video_compress" json:"videoCompress"`
	OwnerID         *int64           `db:"owner_id" json:"ownerId,omitempty"`
	CreatedAt       time.Time        `db:"created_at" json:"createdAt"`
	UpdatedAt       time.Time        `db:"updated_at" json:"updatedAt"`
	DeletedAt       *time.Time       `db:"deleted_at" json:"deletedAt,omitempty"`
}

// AudioTranscription 语音转写任务表行（对应 audio_transcriptions 表）。
type AudioTranscription struct {
	ID         int64            `db:"id" json:"id"`
	ResourceID int64            `db:"resource_id" json:"resourceId"`
	Status     AudioTransStatus `db:"status" json:"status"`
	Text       *string          `db:"text" json:"text,omitempty"`
	ErrorMsg   *string          `db:"error_msg" json:"errorMsg,omitempty"`
	CreatedAt  time.Time        `db:"created_at" json:"createdAt"`
	UpdatedAt  time.Time        `db:"updated_at" json:"updatedAt"`
}

// ToUploadResp 转换为文件上传响应 DTO。
func (r *Resource) ToUploadResp() api.UtilsUploadResp {
	return api.UtilsUploadResp{
		ID:             int(r.ID),
		ResourceID:     &r.ResourceID,
		ResourceSize:   int(r.ResourceSize),
		ResourceSuffix: r.ResourceSuffix,
		ResourceType:   string(r.MediaType),
	}
}

// ToTranscriptionsResp 转换为语音转写响应 DTO。
func (a *AudioTranscription) ToTranscriptionsResp() api.UtilsTranscriptionsResp {
	return api.UtilsTranscriptionsResp{
		Text: ptrStrValue(a.Text),
	}
}
