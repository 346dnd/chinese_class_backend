package repository

import (
	"context"
	"fmt"

	"zhonghuawenhua_backend/internal/model"
)

// ResourceRepository 文件资源数据访问。
type ResourceRepository struct {
	*Base
}

// NewResourceRepository 创建 ResourceRepository。
func NewResourceRepository(base *Base) *ResourceRepository {
	return &ResourceRepository{Base: base}
}

// Create 插入资源，返回新生成的 id。
func (r *ResourceRepository) Create(ctx context.Context, m *model.Resource) (int64, error) {
	const q = `INSERT INTO resources (resource_id, media_type, file_name, file_url, resource_suffix,
		resource_size, resource_info, resource_source, upload_oss, image_compress, video_compress, owner_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) RETURNING id`
	var id int64
	err := r.QueryRow(ctx, &id, q,
		m.ResourceID, m.MediaType, m.FileName, m.FileURL, m.ResourceSuffix,
		m.ResourceSize, m.ResourceInfo, m.ResourceSource, m.UploadOSS,
		m.ImageCompress, m.VideoCompress, m.OwnerID,
	)
	if err != nil {
		return 0, fmt.Errorf("resource create: %w", err)
	}
	return id, nil
}

// GetByID 按主键查询未删除资源。
func (r *ResourceRepository) GetByID(ctx context.Context, id int64) (*model.Resource, error) {
	const q = `SELECT id, resource_id, media_type, file_name, file_url, resource_suffix,
		resource_size, resource_info, resource_source, upload_oss, image_compress, video_compress,
		owner_id, created_at, updated_at, deleted_at
		FROM resources WHERE id = $1 AND deleted_at IS NULL`
	var res model.Resource
	if err := r.QueryRow(ctx, &res, q, id); err != nil {
		return nil, fmt.Errorf("resource get by id: %w", err)
	}
	return &res, nil
}

// GetByResourceID 按业务 resource_id 查询未删除资源。
func (r *ResourceRepository) GetByResourceID(ctx context.Context, resourceID string) (*model.Resource, error) {
	const q = `SELECT id, resource_id, media_type, file_name, file_url, resource_suffix,
		resource_size, resource_info, resource_source, upload_oss, image_compress, video_compress,
		owner_id, created_at, updated_at, deleted_at
		FROM resources WHERE resource_id = $1 AND deleted_at IS NULL`
	var res model.Resource
	if err := r.QueryRow(ctx, &res, q, resourceID); err != nil {
		return nil, fmt.Errorf("resource get by resource_id: %w", err)
	}
	return &res, nil
}

// Update 更新资源可变字段。
func (r *ResourceRepository) Update(ctx context.Context, m *model.Resource) error {
	const q = `UPDATE resources SET file_name = $1, file_url = $2, resource_info = $3,
		resource_source = $4, upload_oss = $5, image_compress = $6, video_compress = $7, owner_id = $8
		WHERE id = $9 AND deleted_at IS NULL`
	if err := r.Exec(ctx, q,
		m.FileName, m.FileURL, m.ResourceInfo, m.ResourceSource,
		m.UploadOSS, m.ImageCompress, m.VideoCompress, m.OwnerID, m.ID,
	); err != nil {
		return fmt.Errorf("resource update: %w", err)
	}
	return nil
}

// Delete 软删除资源。
func (r *ResourceRepository) Delete(ctx context.Context, id int64) error {
	const q = `UPDATE resources SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL`
	if err := r.Exec(ctx, q, id); err != nil {
		return fmt.Errorf("resource delete: %w", err)
	}
	return nil
}

// ListByOwner 按所有者分页查询资源列表。
func (r *ResourceRepository) ListByOwner(ctx context.Context, ownerID int64, limit, offset int) ([]model.Resource, error) {
	const q = `SELECT id, resource_id, media_type, file_name, file_url, resource_suffix,
		resource_size, resource_info, resource_source, upload_oss, image_compress, video_compress,
		owner_id, created_at, updated_at, deleted_at
		FROM resources WHERE owner_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC LIMIT $2 OFFSET $3`
	var list []model.Resource
	if err := r.Query(ctx, &list, q, ownerID, limit, offset); err != nil {
		return nil, fmt.Errorf("resource list by owner: %w", err)
	}
	return list, nil
}

// AudioTranscriptionRepository 语音转写任务数据访问。
type AudioTranscriptionRepository struct {
	*Base
}

// NewAudioTranscriptionRepository 创建 AudioTranscriptionRepository。
func NewAudioTranscriptionRepository(base *Base) *AudioTranscriptionRepository {
	return &AudioTranscriptionRepository{Base: base}
}

// Create 插入转写任务，返回新生成的 id。
func (r *AudioTranscriptionRepository) Create(ctx context.Context, m *model.AudioTranscription) (int64, error) {
	const q = `INSERT INTO audio_transcriptions (resource_id, status, text, error_msg)
		VALUES ($1, $2, $3, $4) RETURNING id`
	var id int64
	err := r.QueryRow(ctx, &id, q, m.ResourceID, m.Status, m.Text, m.ErrorMsg)
	if err != nil {
		return 0, fmt.Errorf("audio_transcription create: %w", err)
	}
	return id, nil
}

// GetByResourceID 按资源 ID 查询转写任务。
func (r *AudioTranscriptionRepository) GetByResourceID(ctx context.Context, resourceID int64) (*model.AudioTranscription, error) {
	const q = `SELECT id, resource_id, status, text, error_msg, created_at, updated_at
		FROM audio_transcriptions WHERE resource_id = $1`
	var a model.AudioTranscription
	if err := r.QueryRow(ctx, &a, q, resourceID); err != nil {
		return nil, fmt.Errorf("audio_transcription get by resource: %w", err)
	}
	return &a, nil
}

// Update 更新转写任务可变字段。
func (r *AudioTranscriptionRepository) Update(ctx context.Context, m *model.AudioTranscription) error {
	const q = `UPDATE audio_transcriptions SET status = $1, text = $2, error_msg = $3 WHERE id = $4`
	if err := r.Exec(ctx, q, m.Status, m.Text, m.ErrorMsg, m.ID); err != nil {
		return fmt.Errorf("audio_transcription update: %w", err)
	}
	return nil
}

// ListByStatus 按状态查询转写任务列表，限制返回数量。
func (r *AudioTranscriptionRepository) ListByStatus(ctx context.Context, status model.AudioTransStatus, limit int) ([]model.AudioTranscription, error) {
	const q = `SELECT id, resource_id, status, text, error_msg, created_at, updated_at
		FROM audio_transcriptions WHERE status = $1 ORDER BY id ASC LIMIT $2`
	var list []model.AudioTranscription
	if err := r.Query(ctx, &list, q, status, limit); err != nil {
		return nil, fmt.Errorf("audio_transcription list by status: %w", err)
	}
	return list, nil
}

// Delete 按主键删除转写任务。
func (r *AudioTranscriptionRepository) Delete(ctx context.Context, id int64) error {
	const q = `DELETE FROM audio_transcriptions WHERE id = $1`
	if err := r.Exec(ctx, q, id); err != nil {
		return fmt.Errorf("audio_transcription delete: %w", err)
	}
	return nil
}
