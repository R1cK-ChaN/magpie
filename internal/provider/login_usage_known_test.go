package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/catalog"
)

// CodexUsedUpKnown is what the catalog sync asks (agent.codexUsedUp), and it
// asks the vendor nothing. SyncCatalog runs inside a write's own request — a
// routing group's Save answers only once every agent has synced — and a
// reading there is one the vendor answers over the network: a save waited
// 1.1s of the reading of the account by the miss of the minute's cache.
// KeepOnAnAccountWithRoom makes the reading, on a loop that has the time for
// it, and noteCodexUsedUp tells the catalog when the answer changes. What a
// reading has already is answered here all the same.
func TestCodexUsedUpKnownAsksTheVendorNothing(t *testing.T) {
	signIn(t)
	rememberLogins(true)
	body := heldUsage(t)
	var hits atomic.Int64
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		json.NewEncoder(w).Encode(body)
	}))
	defer fake.Close()
	old := CodexBase
	CodexBase = fake.URL + "/backend-api/codex"
	t.Cleanup(func() { CodexBase = old })
	loginUsageCache.Lock()
	loginUsageCache.m, loginUsageCache.pending = nil, nil
	loginUsageCache.Unlock()

	// nothing read yet: the account is not known to be held, and no one is
	// asked — CodexUsedUp's own answer for one it can't tell about
	if CodexUsedUpKnown() {
		t.Fatal("CodexUsedUpKnown = true with nothing read")
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("CodexUsedUpKnown asked the vendor %d times, want none", n)
	}
	// the reading the loop makes: it asks the vendor, and the account is held
	if !CodexUsedUp(context.Background()) {
		t.Fatal("CodexUsedUp = false on a held reading")
	}
	if hits.Load() == 0 {
		t.Fatal("CodexUsedUp asked the vendor nothing")
	}
	// and known answers with that reading, asking no one again
	before := hits.Load()
	if !CodexUsedUpKnown() {
		t.Fatal("CodexUsedUpKnown = false on the reading just made")
	}
	if n := hits.Load(); n != before {
		t.Fatalf("CodexUsedUpKnown asked the vendor %d more times, want none", n-before)
	}
}

// forgetReadings leaves nothing read, in memory or among the readings kept.
func forgetReadings(t *testing.T) {
	t.Helper()
	forget := func() {
		loginUsageCache.Lock()
		loginUsageCache.m, loginUsageCache.pending = nil, nil
		loginUsageCache.Unlock()
		lastQuotas.Lock()
		lastQuotas.m, lastQuotas.loaded = nil, false
		lastQuotas.Unlock()
	}
	forget()
	t.Cleanup(forget)
}

// CodexUsedUpRead tells nothing read from a reading that says not held: the
// first is why a takeover stands, the second is what ends it (#5517). A read
// that failed is nothing read, unless a hiccup kept what was read before.
func TestCodexUsedUpReadTellsNothingFromRoom(t *testing.T) {
	signIn(t)
	rememberLogins(true)
	body := heldUsage(t)
	fail := atomic.Bool{}
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail.Load() {
			http.Error(w, "down", 500)
			return
		}
		json.NewEncoder(w).Encode(body)
	}))
	defer fake.Close()
	old := CodexBase
	CodexBase = fake.URL + "/backend-api/codex"
	t.Cleanup(func() { CodexBase = old })
	forgetReadings(t)

	if CodexUsedUpRead() {
		t.Fatal("read, with nothing read yet")
	}
	fail.Store(true)
	CodexUsedUp(context.Background())
	if CodexUsedUpRead() {
		t.Fatal("read, on a read that failed")
	}
	fail.Store(false)
	loginUsageCache.Lock()
	loginUsageCache.m = nil
	loginUsageCache.Unlock()
	if !CodexUsedUp(context.Background()) || !CodexUsedUpRead() {
		t.Fatal("not read, on the reading just made")
	}

	// a hiccup past that reading's minute keeps it: still read
	fail.Store(true)
	loginUsageCache.Lock()
	for k, e := range loginUsageCache.m {
		e.at = time.Now().Add(-2 * time.Minute)
		loginUsageCache.m[k] = e
	}
	loginUsageCache.Unlock()
	CodexUsedUp(context.Background())
	if !CodexUsedUpRead() {
		t.Fatal("the reading kept through a failed read was lost")
	}
}

// A takeover stands till there is a reading, so a first reading is told even
// when it says what was assumed: a magpie that started during a takeover and
// finds room would otherwise never Sync, and stay on the takeover (#5517).
func TestFirstReadingTellsTheCatalog(t *testing.T) {
	signIn(t)
	rememberLogins(true)
	body := heldUsage(t)
	rl, _ := body["rate_limit"].(map[string]any)
	rl["allowed"] = true // room, the same answer as nothing read
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(body)
	}))
	defer fake.Close()
	old := CodexBase
	CodexBase = fake.URL + "/backend-api/codex"
	t.Cleanup(func() { CodexBase = old })
	forgetReadings(t)
	told := 0
	was := catalog.Changed
	catalog.Changed = func() { told++ }
	t.Cleanup(func() { catalog.Changed = was; codexWasUsedUp.Store(false); codexWasRead.Store(false) })
	codexWasUsedUp.Store(false)
	codexWasRead.Store(false)

	noteCodexUsedUp(CodexUsedUp(context.Background()))
	if told != 1 {
		t.Fatalf("the first reading, of an account with room, was told %d times", told)
	}
	noteCodexUsedUp(CodexUsedUp(context.Background()))
	if told != 1 {
		t.Fatalf("the same reading again was told %d times", told)
	}
}
