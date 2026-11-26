package repository

import (
	"encoding/json"
	"strings"

	"github.com/tigerowo/infinite-canvas/model"
	"gorm.io/gorm"
)

func ListCanvasImageTaskIDsWithEmbeddedImages(excluded map[string]bool, limit int) ([]string, error) {
	db, err := DB()
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 8
	}
	query := db.Model(&model.CanvasImageTask{}).Where(
		"image_url LIKE ? OR COALESCE(image_urls, '') LIKE ? OR LENGTH(COALESCE(response_body, '')) > ?",
		"data:image/%",
		"%data:image/%",
		1024*1024,
	)
	if len(excluded) > 0 {
		query = query.Where("id NOT IN ?", mapKeys(excluded))
	}
	var ids []string
	err = query.Order("created_at ASC").Limit(limit).Pluck("id", &ids).Error
	return ids, err
}

func GetCanvasImageTaskByID(id string) (model.CanvasImageTask, bool, error) {
	db, err := DB()
	if err != nil {
		return model.CanvasImageTask{}, false, err
	}
	var task model.CanvasImageTask
	if err := db.First(&task, "id = ?", strings.TrimSpace(id)).Error; err != nil {
		return model.CanvasImageTask{}, false, nil
	}
	return task, true, nil
}

func UpdateCanvasImageTaskStoredImages(id string, imageURL string, imageURLs []string, mimeType string) error {
	db, err := DB()
	if err != nil {
		return err
	}
	fields := map[string]any{
		"image_url":     imageURL,
		"response_body": "",
		"updated_at":    userConfigTimestamp(),
	}
	if mimeType != "" {
		fields["mime_type"] = mimeType
	}
	if len(imageURLs) > 0 {
		encoded, err := json.Marshal(imageURLs)
		if err != nil {
			return err
		}
		fields["image_urls"] = string(encoded)
	}
	return db.Model(&model.CanvasImageTask{}).Where("id = ?", strings.TrimSpace(id)).Updates(fields).Error
}

func ClearCanvasImageTaskResponseBody(id string) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.Model(&model.CanvasImageTask{}).Where("id = ?", strings.TrimSpace(id)).Updates(map[string]any{
		"response_body": "",
		"updated_at":    userConfigTimestamp(),
	}).Error
}

func SaveCanvasImageTask(task model.CanvasImageTask) (model.CanvasImageTask, error) {
	db, err := DB()
	if err != nil {
		return task, err
	}
	return task, db.Save(&task).Error
}

func UpdateCanvasImageTask(task model.CanvasImageTask) (model.CanvasImageTask, error) {
	db, err := DB()
	if err != nil {
		return task, err
	}

	return task, db.Model(&model.CanvasImageTask{}).
		Where("user_id = ? AND id = ?", task.UserID, task.ID).
		Select("*").
		Updates(&task).Error
}

func GetUserCanvasImageTask(userID string, id string) (model.CanvasImageTask, bool, error) {
	db, err := DB()
	if err != nil {
		return model.CanvasImageTask{}, false, err
	}
	var task model.CanvasImageTask
	err = db.First(&task, "user_id = ? AND id = ?", userID, id).Error
	if err != nil {
		return model.CanvasImageTask{}, false, nil
	}
	return task, true, nil
}

func ListUserCanvasImageTasks(userID string, sources []string, limit int) ([]model.CanvasImageTask, error) {
	db, err := DB()
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 100
	}
	var tasks []model.CanvasImageTask
	query := db.Where("user_id = ?", userID)
	if len(sources) > 0 {
		query = query.Where("source IN ?", sources)
	}
	err = query.
		Where("status IN ?", []string{"queued", "processing", "running", "in_progress"}).
		Order("created_at DESC").
		Limit(limit).
		Find(&tasks).Error
	return tasks, err
}

func BatchUserCanvasImageTasks(userID string, ids []string) ([]model.CanvasImageTask, error) {
	db, err := DB()
	if err != nil {
		return nil, err
	}
	keys := uniqueTrimmedValues(ids...)
	if len(keys) == 0 {
		return []model.CanvasImageTask{}, nil
	}
	var tasks []model.CanvasImageTask
	err = db.Where("user_id = ? AND id IN ?", userID, keys).Find(&tasks).Error
	return tasks, err
}

// FailStaleCanvasImageTasks marks unfinished image tasks older than the cutoff as
// failed. A process restart drops the in-memory goroutine that owns a task, so
// without this sweep the task would poll "processing" forever.
func FailStaleCanvasImageTasks(cutoff string, message string) (int64, error) {
	db, err := DB()
	if err != nil {
		return 0, err
	}
	cutoff = strings.TrimSpace(cutoff)
	if cutoff == "" {
		return 0, nil
	}
	result := db.Model(&model.CanvasImageTask{}).
		Where("status IN ?", []string{"queued", "processing", "running", "in_progress"}).
		Where("COALESCE(NULLIF(updated_at, ''), created_at) < ?", cutoff).
		Updates(map[string]any{
			"status":       "failed",
			"completed_at": userConfigTimestamp(),
			"updated_at":   userConfigTimestamp(),
			"error":        firstNonEmptyCanvasTaskValue(message, "图片任务超时未完成"),
			"error_detail": "图片任务在服务重启或上游长时间无响应后中断，请重新生成",
		})
	return result.RowsAffected, result.Error
}

func firstNonEmptyCanvasTaskValue(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func DeleteUserCanvasImageTask(userID string, id string) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.Where("user_id = ? AND id = ?", userID, strings.TrimSpace(id)).Delete(&model.CanvasImageTask{}).Error
}

func DeleteUserCanvasTasks(userID string, sourceID string, nodeIDs []string) error {
	db, err := DB()
	if err != nil {
		return err
	}

	nodeIDs = uniqueTrimmedValues(nodeIDs...)

	return db.Transaction(func(tx *gorm.DB) error {
		deleteTasks := func(task any) error {
			query := tx.Where(
				"user_id = ? AND source = ? AND source_id = ?",
				userID,
				"canvas",
				sourceID,
			)
			if len(nodeIDs) > 0 {
				query = query.Where("node_id IN ?", nodeIDs)
			}
			return query.Delete(task).Error
		}

		if err := deleteTasks(&model.CanvasImageTask{}); err != nil {
			return err
		}
		return deleteTasks(&model.CanvasAudioTask{})
	})
}

func uniqueTrimmedValues(values ...string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			result = append(result, value)
			seen[value] = true
		}
	}
	return result
}

func mapKeys(values map[string]bool) []string {
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	return result
}
