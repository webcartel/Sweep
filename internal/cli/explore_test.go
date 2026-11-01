package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/webcartel/sweep/internal/platform"
	"github.com/webcartel/sweep/internal/platform/fake"
)

// exploreMachine is a fake machine with one folder that holds the most.
func exploreMachine(t *testing.T) deps {
	t.Helper()
	home := `C:\Users\probe`
	fsys := fake.NewFS()
	fsys.Add(platform.Info{Path: home, IsDir: true})
	fsys.Add(platform.Info{Path: home + `\big`, IsDir: true})
	fsys.Add(platform.Info{Path: home + `\big\a.bin`, Size: 100})
	fsys.Add(platform.Info{Path: home + `\small`, IsDir: true})
	fsys.Add(platform.Info{Path: home + `\small\c.bin`, Size: 10})

	env := fake.NewEnv()
	env.Set("USERPROFILE", home)
	return deps{FS: fsys, Env: env, System: fake.NewSystem(), Cwd: home, LogDir: t.TempDir()}
}

// runExplore runs `sweep explore ...` on the deps given.
func runExplore(t *testing.T, d deps, args ...string) (string, string, error) {
	t.Helper()
	argv := pinEnglish(append([]string{"explore"}, args...))
	cmd := newRootCmd(d, argv)
	cmd.SetArgs(argv)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

// The command prints the tree, the totals and the largest files.
func TestExplorePrintsTheTree(t *testing.T) {
	out, errOut, err := runExplore(t, exploreMachine(t), "--depth", "2", "--top", "2")
	if err != nil {
		t.Fatalf("sweep explore: %v", err)
	}
	if errOut != "" {
		t.Errorf("stderr = %q, want nothing", errOut)
	}
	for _, want := range []string{"NAME", "big", "small", "a.bin", "Total:"} {
		if !strings.Contains(out, want) {
			t.Fatalf("the table does not hold %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "110 B") && !strings.Contains(out, "110") {
		t.Errorf("the total of the folder is not printed:\n%s", out)
	}
}

// Without a path the home folder is measured.
func TestExploreUsesTheHomeFolder(t *testing.T) {
	out, _, err := runExplore(t, exploreMachine(t))
	if err != nil {
		t.Fatalf("sweep explore: %v", err)
	}
	if !strings.Contains(out, "big") || !strings.Contains(out, "small") {
		t.Fatalf("the home folder is not measured:\n%s", out)
	}
}

// The JSON document carries the tree and the largest files.
func TestExploreJSON(t *testing.T) {
	out, _, err := runExplore(t, exploreMachine(t), "--json", "--depth", "2", "--top", "1")
	if err != nil {
		t.Fatalf("sweep explore --json: %v", err)
	}
	var answer exploreAnswer
	if err := json.Unmarshal([]byte(out), &answer); err != nil {
		t.Fatalf("%q is not one document: %v", out, err)
	}
	if answer.Schema != 1 {
		t.Fatalf("schema = %d, want 1", answer.Schema)
	}
	if answer.Root.Size != 110 || len(answer.Root.Kids) != 2 {
		t.Fatalf("root = %+v", answer.Root)
	}
	if len(answer.Top) != 1 || answer.Top[0].Name != "a.bin" {
		t.Fatalf("top = %+v, want the one largest file", answer.Top)
	}
}

// A command that answers a parser stays English whatever the interface says.
func TestExploreJSONIsEnglish(t *testing.T) {
	out, _, err := runExplore(t, exploreMachine(t), "--json", "--lang", "ru")
	if err != nil {
		t.Fatalf("sweep explore --json: %v", err)
	}
	if strings.Contains(out, "ИМЯ") || strings.Contains(out, "Всего") {
		t.Fatalf("the JSON document is translated: %s", out)
	}
}
