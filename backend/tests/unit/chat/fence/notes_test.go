// Scope fence: user-written notes never reach the agent, since in the prompt
// they would be an injection path. Ported from the Python notes isolation test.
package fence_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"urara-vision/backend/internal/chat/apiclient"
	"urara-vision/backend/internal/chat/tools"
)

func TestTheBackendClientHasNoNotesMethod(t *testing.T) {
	c := reflect.TypeFor[*apiclient.Client]()
	for i := range c.NumMethod() {
		if name := c.Method(i).Name; strings.Contains(strings.ToLower(name), "note") {
			t.Errorf("apiclient.Client.%s", name)
		}
	}
}

func TestNoToolMentionsNotes(t *testing.T) {
	for _, s := range tools.Build(nil, "s1") {
		if strings.Contains(strings.ToLower(s.Name+" "+s.Description), "note") {
			t.Errorf("tool %s mentions notes", s.Name)
		}
	}
}

func TestNoChatSourceCallsTheNotesRoutes(t *testing.T) {
	root := filepath.Join("..", "..", "..", "..", "internal", "chat")
	seen := 0
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		seen++
		src, err := os.ReadFile(p)
		if err == nil && strings.Contains(string(src), "/notes") {
			t.Errorf("%s calls a notes route", p)
		}
		return err
	})
	if err != nil || seen == 0 {
		t.Fatalf("walked %d files: %v", seen, err)
	}
}
