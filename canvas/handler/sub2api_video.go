package handler

import (
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/tigerowo/infinite-canvas/service"
)

func serveSub2APIVideoContent(w http.ResponseWriter, r *http.Request, id string) bool {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		return false
	}
	task, found, err := service.GetUserVideoTask(user.ID, id)
	if err != nil {
		FailWithStatus(w, 500, "读取视频任务失败")
		return true
	}
	if !found || task.ChannelID != "sub2api-relay" || task.UserChannelID != "" {
		return false
	}
	if task.Status != "completed" {
		FailWithStatus(w, 409, "视频尚未完成")
		return true
	}
	channel, err := service.SelectModelChannelForModel(task.Model, task.ChannelID)
	if err != nil {
		FailWithStatus(w, 503, "视频渠道不可用")
		return true
	}
	upstreamID := firstNonEmpty(task.UpstreamTaskID, task.UpstreamVideoID)
	if upstreamID == "" {
		FailWithStatus(w, 502, "视频缺少上游任务 ID")
		return true
	}
	request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, service.BuildModelChannelURL(channel, "/videos/"+url.PathEscape(upstreamID)+"/content"), nil)
	if err != nil {
		FailWithStatus(w, 502, "视频内容请求失败")
		return true
	}
	service.SetModelChannelAuthHeader(request, channel)
	response, err := service.HTTPClientForChannel(channel).Do(request)
	if err != nil {
		FailWithStatus(w, 502, "视频内容下载失败")
		return true
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		FailWithStatus(w, response.StatusCode, "视频内容暂不可用")
		return true
	}
	contentType := response.Header.Get("Content-Type")
	if !strings.HasPrefix(strings.ToLower(contentType), "video/") {
		FailWithStatus(w, 502, "上游未返回视频内容")
		return true
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "private, no-store")
	_, _ = io.Copy(w, response.Body)
	return true
}
