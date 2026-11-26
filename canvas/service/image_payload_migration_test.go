package service

import (
	"strings"
	"testing"

	"github.com/tigerowo/infinite-canvas/model"
)

func TestMaterializeTaskImagePayloadsRewritesDuplicateURLArray(t *testing.T) {
	t.Setenv("GENERATED_IMAGE_DIR", t.TempDir())
	const imageData = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAAC0lEQVR4nGP4DwQACfsD/fteaysAAAAASUVORK5CYII="
	task := model.CanvasImageTask{ID: "task-1", UserID: "user-1", ImageURL: imageData, ImageURLs: []string{imageData}}

	imageURL, imageURLs, _, err := materializeTaskImagePayloads(task)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(imageURL, generatedImageURLPrefix) || len(imageURLs) != 1 || imageURLs[0] != imageURL {
		t.Fatalf("unexpected migration result: imageURL=%q imageURLs=%v", imageURL, imageURLs)
	}
}
