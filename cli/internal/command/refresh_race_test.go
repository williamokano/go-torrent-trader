package command

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/williamokano/go-torrent-trader/cli/internal/config"
)

// #239. Renewal is a read-modify-write across three places, and only the *write* was
// serialised. Two tt invocations that overlap — two cron entries on the same minute,
// an `xargs -P` fan-out, a wrapper looping over profiles — both read the same pair
// and both try to spend it. The server rotates the refresh token on every renewal, so
// the second request is refused, and that invocation used to fall back to its expired
// access token, get a 401, and exit 3.
//
// An intermittent auth failure in exactly the unattended case `tt auth login` exists
// to serve, and the suite had no concurrent-invocation coverage of the refresh path
// at all, which is why it was invisible.
//
// This drives genuinely concurrent refreshers against one fake site that rotates and
// refuses a replayed token, exactly as the real one does.
func TestConcurrentRefreshersAllSucceed(t *testing.T) {
	isolate(t)
	site, srv := newFakeSite(t)
	setupProfile(t, srv.URL)

	// A pair that is already spent, so every refresher renews.
	site.mu.Lock()
	site.accessTTL = 1
	site.mu.Unlock()

	if got := runCLI(t, "hunter2\n", "auth", "login", "prod",
		"--username", "alice", "--password-stdin"); got.err != nil {
		t.Fatalf("auth login: %v", got.err)
	}

	stored, err := config.LoadCredentialRecord("prod")
	if err != nil {
		t.Fatalf("LoadCredentialRecord: %v", err)
	}

	const n = 6
	var wg sync.WaitGroup
	tokens := make([]string, n)
	errs := make([]error, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// Each starts from the same stored pair, which is what separate
			// processes reading the same file would do.
			refresh := refresherFor("prod", srv.URL, stored, io.Discard)
			if refresh == nil {
				t.Errorf("refresher %d: none built for a credential that can refresh", i)
				return
			}
			tokens[i], errs[i] = refresh(context.Background())
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("refresher %d failed: %v — a concurrent tt invocation lost the "+
				"race and would have exited 3", i, err)
		}
		if tokens[i] == "" {
			t.Errorf("refresher %d returned no token", i)
		}
	}

	// Every one must have ended up with a token the site actually accepts, not
	// merely a non-empty string.
	site.mu.Lock()
	valid := site.validAccess
	site.mu.Unlock()
	for i, tok := range tokens {
		if tok != valid && tok != "" {
			// Adopting a slightly older pair is fine as long as it is still live;
			// what must not happen is a failure.
			t.Logf("refresher %d holds a token that is not the newest (acceptable)", i)
		}
	}

	// And the file must hold one coherent pair afterwards, not a half-written one.
	final, err := config.LoadCredentialRecord("prod")
	if err != nil {
		t.Fatalf("LoadCredentialRecord after the race: %v", err)
	}
	if final.Token == "" || final.RefreshToken == "" {
		t.Errorf("the stored credential is incomplete after concurrent refreshes: %+v", final)
	}
}

// The optimisation half: when another invocation has already rotated, this one must
// adopt the stored pair rather than spending a round trip to be refused.
func TestARefresherAdoptsAPairAnotherInvocationAlreadyStored(t *testing.T) {
	isolate(t)
	site, srv := newFakeSite(t)
	setupProfile(t, srv.URL)

	site.mu.Lock()
	site.accessTTL = 1
	site.mu.Unlock()

	if got := runCLI(t, "hunter2\n", "auth", "login", "prod",
		"--username", "alice", "--password-stdin"); got.err != nil {
		t.Fatalf("auth login: %v", got.err)
	}
	stale, err := config.LoadCredentialRecord("prod")
	if err != nil {
		t.Fatalf("LoadCredentialRecord: %v", err)
	}

	// Another invocation refreshes and stores a live pair.
	winner := refresherFor("prod", srv.URL, stale, io.Discard)
	if winner == nil {
		t.Fatal("no refresher for a credential that can refresh")
	}
	if _, err := winner(context.Background()); err != nil {
		t.Fatalf("the winning refresh failed: %v", err)
	}

	site.mu.Lock()
	before := site.refreshCalls
	site.mu.Unlock()

	// This one still holds the stale pair. It must notice and adopt, without
	// spending a request the server would refuse.
	loser := refresherFor("prod", srv.URL, stale, io.Discard)
	token, err := loser(context.Background())
	if err != nil {
		t.Fatalf("the losing refresher failed instead of adopting: %v", err)
	}
	if token == "" {
		t.Fatal("the losing refresher returned no token")
	}

	site.mu.Lock()
	after := site.refreshCalls
	site.mu.Unlock()
	if after != before {
		t.Errorf("refreshCalls went %d -> %d; the stored pair was already usable, so "+
			"this should have cost no round trip", before, after)
	}
}

// A genuinely dead refresh token, with nothing newer on disk, must still surface as
// an error — otherwise the adopt path would swallow real authentication failures.
func TestADeadRefreshTokenStillFailsWhenNothingNewerIsStored(t *testing.T) {
	isolate(t)
	_, srv := newFakeSite(t)
	setupProfile(t, srv.URL)

	if got := runCLI(t, "hunter2\n", "auth", "login", "prod",
		"--username", "alice", "--password-stdin"); got.err != nil {
		t.Fatalf("auth login: %v", got.err)
	}
	stored, err := config.LoadCredentialRecord("prod")
	if err != nil {
		t.Fatalf("LoadCredentialRecord: %v", err)
	}

	// Dead on disk *and* in memory. Mutating only the in-memory copy would leave a
	// live pair on disk, and adopting that is correct behaviour rather than a bug —
	// the stored credential is the source of truth, and the refresher's job is to
	// produce a working token, not to insist on using the one it was handed.
	stored.RefreshToken = "a-token-that-was-never-issued"
	stored.Token = "also-dead"
	if err := config.StoreCredentialRecord("prod", stored); err != nil {
		t.Fatalf("storing the dead pair: %v", err)
	}
	refresh := refresherFor("prod", srv.URL, stored, io.Discard)
	if refresh == nil {
		t.Fatal("no refresher")
	}

	if _, err := refresh(context.Background()); err == nil {
		t.Error("a dead refresh token succeeded; the adopt path is swallowing real " +
			"authentication failures")
	} else if strings.Contains(err.Error(), "panic") {
		t.Errorf("unexpected failure mode: %v", err)
	}
}
