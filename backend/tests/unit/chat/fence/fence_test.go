// Scope fence: the chat binary must never reach a store, the graph, the parser,
// the backend's API package or a database driver.
package fence_test

import (
	"os/exec"
	"strings"
	"testing"
)

const chatPkg = "urara-vision/backend/cmd/chat"

var forbidden = []string{
	"urara-vision/backend/internal/store",
	"urara-vision/backend/internal/graph",
	"urara-vision/backend/internal/parser",
	"urara-vision/backend/internal/api",
	"github.com/jackc/pgx",
	"github.com/neo4j/neo4j-go-driver",
}

func goList(t *testing.T, args ...string) string {
	t.Helper()
	out, err := exec.Command("go", append([]string{"list"}, args...)...).Output()
	if err != nil {
		t.Fatalf("go list %v: %v", args, err)
	}
	return string(out)
}

// banned reports whether pkg is, or sits under, a forbidden path.
func banned(pkg string) bool {
	for _, f := range forbidden {
		if pkg == f || strings.HasPrefix(pkg, f+"/") {
			return true
		}
	}
	return false
}

func TestChatDependsOnNoStoreOrDriver(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("the go binary is not on PATH, so dependencies cannot be listed")
	}
	imports := map[string][]string{}
	out := goList(t, "-f", `{{.ImportPath}}: {{join .Imports " "}}`, "-deps", chatPkg)
	for _, line := range strings.Split(out, "\n") {
		if pkg, deps, ok := strings.Cut(line, ":"); ok {
			imports[pkg] = strings.Fields(deps)
		}
	}
	if len(imports) == 0 {
		t.Fatal("go list returned no packages")
	}

	// Breadth-first from the binary, so the offender named is the one nearest to chat code.
	prev := map[string]string{chatPkg: ""}
	queue := []string{chatPkg}
	for len(queue) > 0 {
		pkg := queue[0]
		queue = queue[1:]
		if banned(pkg) {
			var chain []string
			for p := pkg; p != ""; p = prev[p] {
				chain = append([]string{p}, chain...)
			}
			t.Fatalf("%s depends on %s\nimport chain: %s", chatPkg, pkg, strings.Join(chain, " -> "))
		}
		for _, dep := range imports[pkg] {
			if _, seen := prev[dep]; !seen {
				prev[dep] = pkg
				queue = append(queue, dep)
			}
		}
	}
}
