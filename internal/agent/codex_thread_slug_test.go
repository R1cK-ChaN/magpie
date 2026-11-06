package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/yetone/magpie/internal/provider"
)

// When magpie becomes Codex's provider for a used-up account, a thread from
// before is still on the slug OpenAI gave its model. The catalog magpie
// writes then describes that slug, hidden, as it does codex/<slug> (#5504).
func TestCodexTakeoverDescribesAThreadsSlug(t *testing.T) {
	home, read := codexHome(t, `{"tokens":{"access_token":"x","id_token":"x.e30.x"}}`, "model = \"gpt-a\"\n")
	if err := provider.Save(provider.Provider{ID: "codex", Fallback: []string{"fake/m1"}}); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(home, ".codex", "models_cache.json"),
		[]byte(`{"models":[{"slug":"gpt-a","display_name":"A","priority":1,"tool_mode":"code_mode_only"}]}`), 0o644)
	cx := codex(home)
	if ok, err := cx.Join(); err != nil || !ok {
		t.Fatalf("join: %v %v", ok, err)
	}
	codexUsedUp = func() bool { return true }
	if err := cx.Sync(); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(home, ".codex", "magpie-models.json"))
	if err != nil {
		t.Fatalf("%v\n%s", err, read())
	}
	var list struct {
		Models []map[string]any `json:"models"`
	}
	if err := json.Unmarshal(b, &list); err != nil {
		t.Fatal(err)
	}
	by := map[string]map[string]any{}
	for _, m := range list.Models {
		by[m["slug"].(string)] = m
	}
	thread, listed := by["gpt-a"], by["codex/gpt-a"]
	if listed == nil || thread == nil {
		t.Fatalf("no entry for the thread's slug: %d entries", len(list.Models))
	}
	if thread["visibility"] != "hide" || thread["tool_mode"] != "code_mode_only" {
		t.Fatalf("the thread's slug: visibility %v, tool_mode %v", thread["visibility"], thread["tool_mode"])
	}
}
