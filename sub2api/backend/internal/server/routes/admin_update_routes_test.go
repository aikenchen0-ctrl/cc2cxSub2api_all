package routes

import (
	"os"
	"strings"
	"testing"
)

func TestAdminSystemRoutesAreReadOnly(t *testing.T) {
	source, err := os.ReadFile("admin.go")
	if err != nil {
		t.Fatalf("read admin routes: %v", err)
	}

	routes := string(source)
	for _, required := range []string{
		`system.GET("/version"`,
		`system.GET("/check-updates"`,
	} {
		if !strings.Contains(routes, required) {
			t.Errorf("missing read-only system route %s", required)
		}
	}

	for _, forbidden := range []string{
		`system.POST("/update"`,
		`system.POST("/rollback"`,
		`system.GET("/rollback-versions"`,
	} {
		if strings.Contains(routes, forbidden) {
			t.Errorf("version-changing route must not be registered: %s", forbidden)
		}
	}
}
