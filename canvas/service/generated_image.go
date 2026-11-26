package service

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const generatedImageURLPrefix = "/api/v1/canvas/image-tasks/"

var generatedImageKeyPattern = regexp.MustCompile(`^[a-f0-9]{32}\.(png|jpg|webp|gif|bin)$`)

func generatedImageRoot() string {
	if value := strings.TrimSpace(os.Getenv("GENERATED_IMAGE_DIR")); value != "" {
		return value
	}
	return filepath.Join("data", "generated-images")
}

func generatedImageDirectory(userID string, taskID string) string {
	userHash := sha256.Sum256([]byte(strings.TrimSpace(userID)))
	taskHash := sha256.Sum256([]byte(strings.TrimSpace(taskID)))
	return filepath.Join(generatedImageRoot(), hex.EncodeToString(userHash[:8]), hex.EncodeToString(taskHash[:16]))
}

func generatedImageExtension(contentType string) string {
	switch strings.ToLower(strings.Split(contentType, ";")[0]) {
	case "image/png":
		return ".png"
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	default:
		return ".bin"
	}
}

// SaveGeneratedImage stores a generated image in the data volume and returns a short, authenticated URL.
func SaveGeneratedImage(userID string, taskID string, data []byte, contentType string) (string, error) {
	if len(data) == 0 {
		return "", errors.New("图片内容为空")
	}
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(taskID) == "" {
		return "", errors.New("图片任务标识无效")
	}
	sum := sha256.Sum256(data)
	key := hex.EncodeToString(sum[:16]) + generatedImageExtension(contentType)
	directory := generatedImageDirectory(userID, taskID)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return "", err
	}
	filename := filepath.Join(directory, key)
	if info, err := os.Stat(filename); err == nil && info.Size() == int64(len(data)) {
		return generatedImageURL(taskID, key), nil
	}
	temp := filename + ".tmp"
	if err := os.WriteFile(temp, data, 0o644); err != nil {
		return "", err
	}
	if err := os.Rename(temp, filename); err != nil {
		_ = os.Remove(temp)
		return "", err
	}
	return generatedImageURL(taskID, key), nil
}

func generatedImageURL(taskID string, key string) string {
	return generatedImageURLPrefix + url.PathEscape(strings.TrimSpace(taskID)) + "/content?key=" + url.QueryEscape(key)
}

func CanvasGeneratedImagePath(userID string, taskID string, key string) (string, error) {
	key = strings.ToLower(strings.TrimSpace(key))
	if !generatedImageKeyPattern.MatchString(key) {
		return "", errors.New("图片标识无效")
	}
	return filepath.Join(generatedImageDirectory(userID, taskID), key), nil
}
