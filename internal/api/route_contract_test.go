package api

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"
)

// TestEveryRESTRouteDocumented checks both directions, including HTTP methods.
// server.go is the registration source; no server or camera is started here.
func TestEveryRESTRouteDocumented(t *testing.T) {
	source, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	var spec struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal([]byte(openAPISpec), &spec); err != nil {
		t.Fatal(err)
	}
	routes := map[string]bool{}
	params := regexp.MustCompile(`:([A-Za-z_][A-Za-z_0-9]*)`)
	for _, match := range regexp.MustCompile(`api\.(GET|POST|PUT|PATCH|DELETE)\("([^"]+)"`).FindAllStringSubmatch(string(source), -1) {
		method, path := strings.ToLower(match[1]), params.ReplaceAllString(match[2], "{$1}")
		routes[method+" "+path] = true
		if _, ok := spec.Paths[path][method]; !ok {
			t.Errorf("undocumented route: %s %s", method, path)
		}
	}
	for path, methods := range spec.Paths {
		for method := range methods {
			switch method {
			case "get", "post", "put", "patch", "delete":
				if !routes[method+" "+path] {
					t.Errorf("documented route not registered: %s %s", method, path)
				}
			}
		}
	}
}
