package codexcat

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/yetone/magpie/internal/catalog"
)

func decodeCatalog(t *testing.T, b []byte) map[string]map[string]any {
	t.Helper()
	var got struct {
		Models []map[string]any `json:"models"`
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	by := map[string]map[string]any{}
	for _, m := range got.Models {
		by[m["slug"].(string)] = m
	}
	return by
}

// A thread started before magpie became Codex's provider is still on the
// slug OpenAI gave its model. The catalog describes that slug as it does
// codex/<slug>, or Codex takes the thread's model for an unknown one and
// sends every MCP tool inline on every turn (#5504).
func TestCatalogUsedDescribesAThreadsSlug(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	os.MkdirAll(filepath.Join(home, ".codex"), 0o755)
	os.WriteFile(filepath.Join(home, ".codex", "models_cache.json"), []byte(`{"models":[
		{"slug":"gpt-6-astra","display_name":"GPT-6-Astra","priority":1,"visibility":"list",
		 "tool_mode":"code_mode_only","supports_search_tool":true,"context_window":272000,
		 "supported_reasoning_levels":[{"effort":"medium","description":"m"},{"effort":"ultra","description":"u"}]}]}`), 0o644)

	by := decodeCatalog(t, CatalogUsed([]catalog.Model{
		{ID: "deepseek/deepseek-chat", Name: "deepseek-chat · DeepSeek"},
		{ID: "codex/gpt-6-astra", Name: "GPT-6-Astra · Codex"},
		{ID: "codex/gpt-9-unlisted", Name: "GPT-9 · Codex"}, // no entry in Codex's cache
	}, "", nil))
	listed, thread := by["codex/gpt-6-astra"], by["gpt-6-astra"]
	if listed == nil || thread == nil {
		t.Fatalf("slugs %v", slices.Sorted(maps.Keys(by)))
	}
	if thread["visibility"] != "hide" {
		t.Errorf("a picker would show the thread's slug: %v", thread["visibility"])
	}
	if listed["visibility"] != "list" {
		t.Errorf("the listed entry changed: %v", listed["visibility"])
	}
	for _, k := range []string{"tool_mode", "supports_search_tool", "supported_reasoning_levels", "context_window", "base_instructions"} {
		if thread[k] == nil || !reflect.DeepEqual(thread[k], listed[k]) {
			t.Errorf("%s: thread's %v, listed %v", k, thread[k], listed[k])
		}
	}
	if thread["priority"].(float64) <= listed["priority"].(float64) ||
		thread["priority"].(float64) <= by["deepseek/deepseek-chat"]["priority"].(float64) {
		t.Errorf("a hidden entry ranks ahead of a listed one: %v", thread["priority"])
	}
	if e := by["gpt-9-unlisted"]; e == nil || e["visibility"] != "hide" {
		t.Errorf("a ChatGPT model Codex's cache lacks: slugs %v", slices.Sorted(maps.Keys(by)))
	}
	if by["deepseek-chat"] != nil {
		t.Error("a third party's model got a slug of its own")
	}
}

// A model the list has under the bare id is its own and keeps its entry,
// listed.
func TestThreadSlugsLeaveAListedIDAlone(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	by := decodeCatalog(t, CatalogUsed([]catalog.Model{
		{ID: "codex/gpt-6-astra", Name: "GPT-6-Astra · Codex"},
		{ID: "gpt-6-astra", Name: "A relay's GPT-6-Astra"},
	}, "", nil))
	if by["gpt-6-astra"]["visibility"] != "list" || by["gpt-6-astra"]["display_name"] != "A relay's GPT-6-Astra" {
		t.Fatalf("the listed id: %v, %v", by["gpt-6-astra"]["visibility"], by["gpt-6-astra"]["display_name"])
	}
}

// CatalogAt, T3 Code's, is for an instance that starts its own threads on
// magpie's models: it has no slugs from before to describe.
func TestCatalogAtAddsNoThreadSlugs(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	by := decodeCatalog(t, CatalogAt([]catalog.Model{{ID: "codex/gpt-6-astra", Name: "GPT-6-Astra · Codex"}}, ""))
	if len(by) != 1 || by["codex/gpt-6-astra"] == nil {
		t.Fatalf("slugs %v", slices.Sorted(maps.Keys(by)))
	}
}
