package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/provider"
)

// usedUpThenStarted puts Codex on magpie for a used-up account, then has
// the magpie that wired it stand in for a new one: nothing read yet.
func usedUpThenStarted(t *testing.T) (cx *Agent, read func() string) {
	t.Helper()
	home, read := codexHome(t, `{"tokens":{"access_token":"x","id_token":"x.e30.x"}}`, "model = \"gpt-a\"\n")
	if err := provider.Save(provider.Provider{ID: "codex", Fallback: []string{"fake/m1"}}); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(home, ".codex", "models_cache.json"),
		[]byte(`{"models":[{"slug":"gpt-a","display_name":"A","priority":1}]}`), 0o644)
	cx = codex(home)
	if ok, err := cx.Join(); err != nil || !ok {
		t.Fatalf("join: %v %v", ok, err)
	}
	codexUsedUp = func() bool { return true }
	if err := cx.Sync(); err != nil {
		t.Fatal(err)
	}
	if cfg := read(); !strings.Contains(cfg, `model_provider = "magpie"`) || !strings.Contains(cfg, `model = "codex/gpt-a"`) {
		t.Fatalf("used up:\n%s", cfg)
	}
	codexUsedUp = func() bool { return false }
	codexUsedUpRead = func() bool { return false }
	return cx, read
}

// A magpie that starts during a takeover has read nothing of the account
// yet. That isn't room: the takeover stands, and Codex's threads and its
// app stay where they are, till a reading says otherwise (#5517).
func TestCodexTakeoverStandsTillAReading(t *testing.T) {
	cx, read := usedUpThenStarted(t)
	if err := cx.Sync(); err != nil {
		t.Fatal(err)
	}
	cfg := read()
	if !strings.Contains(cfg, `model_provider = "magpie"`) || !strings.Contains(cfg, `model = "codex/gpt-a"`) {
		t.Fatalf("nothing read, and the takeover ended:\n%s", cfg)
	}
	if strings.Contains(cfg, "requires_openai_auth") {
		t.Fatalf("nothing read, and the app was given the used-up account back:\n%s", cfg)
	}
	if !cx.AppOut() {
		t.Fatal("nothing read: not AppOut")
	}

	// a reading that says the account is held changes nothing
	codexUsedUp, codexUsedUpRead = func() bool { return true }, func() bool { return true }
	if err := cx.Sync(); err != nil {
		t.Fatal(err)
	}
	if cfg := read(); !strings.Contains(cfg, `model = "codex/gpt-a"`) {
		t.Fatalf("held:\n%s", cfg)
	}

	// and one that says it has room ends it
	codexUsedUp = func() bool { return false }
	if err := cx.Sync(); err != nil {
		t.Fatal(err)
	}
	cfg = read()
	if strings.Contains(cfg, "model_provider =") || !strings.Contains(cfg, `model = "gpt-a"`) {
		t.Fatalf("room:\n%s", cfg)
	}
	if cx.AppOut() {
		t.Fatal("room: AppOut")
	}
}

// A pick made before the first look goes through the same way: it doesn't
// end the takeover by itself (the model-set path of the same decision).
func TestCodexPickBeforeAReadingKeepsTheTakeover(t *testing.T) {
	cx, read := usedUpThenStarted(t)
	if err := cx.Fields[0].Set("fake/m1"); err != nil {
		t.Fatal(err)
	}
	if cfg := read(); !strings.Contains(cfg, `model_provider = "magpie"`) || !strings.Contains(cfg, `model = "fake/m1"`) {
		t.Fatalf("a pick with nothing read:\n%s", cfg)
	}
	if !cx.AppOut() {
		t.Fatal("a pick with nothing read: not AppOut")
	}
}

// Nothing read matters only to a takeover: Codex beside its sign-in stays
// there, as it isn't told it's out by a reading no one has made.
func TestCodexBesideItsSignInIgnoresNothingRead(t *testing.T) {
	home, read := codexHome(t, `{"tokens":{"access_token":"x","id_token":"x.e30.x"}}`, "model = \"gpt-a\"\n")
	os.WriteFile(filepath.Join(home, ".codex", "models_cache.json"),
		[]byte(`{"models":[{"slug":"gpt-a","display_name":"A","priority":1}]}`), 0o644)
	cx := codex(home)
	if ok, err := cx.Join(); err != nil || !ok {
		t.Fatalf("join: %v %v", ok, err)
	}
	codexUsedUpRead = func() bool { return false }
	if err := cx.Sync(); err != nil {
		t.Fatal(err)
	}
	if cfg := read(); strings.Contains(cfg, "model_provider =") || !strings.Contains(cfg, `model = "gpt-a"`) {
		t.Fatalf("joined, nothing read:\n%s", cfg)
	}
}
