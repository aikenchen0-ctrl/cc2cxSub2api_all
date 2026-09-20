package handler

import "github.com/Wei-Shaw/sub2api/pkg/satellite"

// projectLink resolves {project}_link / PROJECT_LINK from 本地链接.txt.
func projectLink(project, fallback string) string {
	return satellite.ProjectOrigin(project, fallback)
}

func ssoCallback(envName, project, fallbackOrigin, path string) string {
	return satellite.CallbackURL(envName, project, fallbackOrigin, path)
}
