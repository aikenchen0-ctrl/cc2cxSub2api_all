package service

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/tigerowo/infinite-canvas/model"
	"github.com/tigerowo/infinite-canvas/repository"
)

var imagePayloadMigrationOnce sync.Once

// MigrateLegacyImagePayloads moves base64 images out of the database and clears oversized raw responses.
func MigrateLegacyImagePayloads() {
	imagePayloadMigrationOnce.Do(func() {
		tasks, err := migrateCanvasImageTaskPayloads()
		if err != nil {
			log.Printf("canvas image payload migration failed: tasks=%d err=%v", tasks, err)
		}
		logs, err := migrateGenerationLogPayloads()
		if err != nil {
			log.Printf("canvas image payload migration failed: logs=%d err=%v", logs, err)
		}
		log.Printf("canvas image payload migration finished: tasks=%d logs=%d", tasks, logs)
	})
}

func migrateCanvasImageTaskPayloads() (int, error) {
	excluded := map[string]bool{}
	migrated := 0
	for {
		ids, err := repository.ListCanvasImageTaskIDsWithEmbeddedImages(excluded, 8)
		if err != nil {
			return migrated, err
		}
		if len(ids) == 0 {
			return migrated, nil
		}
		progress := false
		for _, id := range ids {
			task, found, err := repository.GetCanvasImageTaskByID(id)
			if err != nil || !found {
				excluded[id] = true
				continue
			}
			if len(task.ImageURLs) == 0 && strings.TrimSpace(task.ImageURL) == "" {
				if err := repository.ClearCanvasImageTaskResponseBody(task.ID); err != nil {
					excluded[id] = true
					log.Printf("canvas image payload migration clear failed task=%s err=%v", id, err)
					continue
				}
				migrated++
				progress = true
				continue
			}
			storedURL, storedURLs, contentType, err := materializeTaskImagePayloads(task)
			if err != nil {
				excluded[id] = true
				log.Printf("canvas image payload migration skipped task=%s err=%v", id, err)
				continue
			}
			if err := repository.UpdateCanvasImageTaskStoredImages(task.ID, storedURL, storedURLs, contentType); err != nil {
				excluded[id] = true
				log.Printf("canvas image payload migration update failed task=%s err=%v", id, err)
				continue
			}
			migrated++
			progress = true
		}
		if !progress {
			return migrated, nil
		}
	}
}

func materializeTaskImagePayloads(task model.CanvasImageTask) (string, []string, string, error) {
	candidates := append([]string{}, task.ImageURLs...)
	if strings.TrimSpace(task.ImageURL) != "" {
		candidates = append([]string{task.ImageURL}, candidates...)
	}
	urls := make([]string, 0, len(candidates))
	contentType := task.MimeType
	seen := map[string]bool{}
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate) == "" {
			continue
		}
		if strings.HasPrefix(candidate, "data:image/") || looksLikeBase64Image(candidate) {
			data, detectedType, err := decodeImagePayload(candidate)
			if err != nil {
				return "", nil, "", err
			}
			if contentType == "" {
				contentType = detectedType
			}
			candidate, err = SaveGeneratedImage(task.UserID, task.ID, data, detectedType)
			if err != nil {
				return "", nil, "", err
			}
		}
		if !seen[candidate] {
			seen[candidate] = true
			urls = append(urls, candidate)
		}
	}
	if len(urls) == 0 {
		return "", nil, "", fmt.Errorf("没有可迁移的图片")
	}
	if len(task.ImageURLs) == 0 {
		return urls[0], nil, contentType, nil
	}
	return urls[0], urls, contentType, nil
}

func migrateGenerationLogPayloads() (int, error) {
	excluded := map[string]bool{}
	migrated := 0
	for {
		logs, err := repository.ListImageGenerationLogPayloadsWithEmbeddedImages(excluded, 4)
		if err != nil {
			return migrated, err
		}
		if len(logs) == 0 {
			return migrated, nil
		}
		progress := false
		for _, generationLog := range logs {
			var payload any
			if err := json.Unmarshal([]byte(generationLog.PayloadJSON), &payload); err != nil {
				excluded[generationLog.ID] = true
				continue
			}
			taskID := generationLog.ID
			if root, ok := payload.(map[string]any); ok {
				if task, ok := root["task"].(map[string]any); ok {
					taskID = firstNonEmpty(toStringValue(task["id"]), taskID)
				}
			}
			nextPayload, err := materializeLogImagePayloads(payload, generationLog.UserID, taskID)
			if err != nil {
				excluded[generationLog.ID] = true
				log.Printf("canvas generation log payload migration skipped log=%s err=%v", generationLog.ID, err)
				continue
			}
			encoded, err := json.Marshal(nextPayload)
			if err != nil {
				excluded[generationLog.ID] = true
				continue
			}
			if err := repository.UpdateImageGenerationLogPayload(generationLog.ID, string(encoded)); err != nil {
				excluded[generationLog.ID] = true
				log.Printf("canvas generation log payload update failed log=%s err=%v", generationLog.ID, err)
				continue
			}
			migrated++
			progress = true
		}
		if !progress {
			return migrated, nil
		}
	}
}

func materializeLogImagePayloads(value any, userID string, taskID string) (any, error) {
	switch typed := value.(type) {
	case string:
		if !strings.HasPrefix(typed, "data:image/") {
			return typed, nil
		}
		data, contentType, err := decodeImagePayload(typed)
		if err != nil {
			return nil, err
		}
		return SaveGeneratedImage(userID, taskID, data, contentType)
	case []any:
		next := make([]any, len(typed))
		for index, item := range typed {
			value, err := materializeLogImagePayloads(item, userID, taskID)
			if err != nil {
				return nil, err
			}
			next[index] = value
		}
		return next, nil
	case map[string]any:
		next := make(map[string]any, len(typed))
		for key, item := range typed {
			value, err := materializeLogImagePayloads(item, userID, taskID)
			if err != nil {
				return nil, err
			}
			next[key] = value
		}
		return next, nil
	default:
		return value, nil
	}
}

func decodeImagePayload(value string) ([]byte, string, error) {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "data:image/") {
		parts := strings.SplitN(value, ",", 2)
		if len(parts) != 2 {
			return nil, "", fmt.Errorf("无效图片 data url")
		}
		contentType := strings.TrimPrefix(strings.Split(strings.TrimPrefix(parts[0], "data:"), ";")[0], " ")
		data, err := base64.StdEncoding.DecodeString(parts[1])
		if err != nil {
			return nil, "", err
		}
		return data, contentType, nil
	}
	data, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil, "", err
	}
	return data, "image/png", nil
}

func looksLikeBase64Image(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) < 256 || len(value)%4 != 0 {
		return false
	}
	return strings.HasPrefix(value, "iVBORw0KGgo") || strings.HasPrefix(value, "/9j/") || strings.HasPrefix(value, "UklGR")
}

func toStringValue(value any) string {
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text)
	}
	return ""
}
