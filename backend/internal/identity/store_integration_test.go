package identity

import (
	"bytes"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/PuraFome/meuRPG/backend/internal/platform/dbtest"
	"github.com/PuraFome/meuRPG/backend/internal/platform/secret"
)

// These tests need CockroachDB: set MEURPG_TEST_DATABASE_URL (see package
// dbtest). Without it they skip, so `go test ./...` works anywhere.

// namedStore builds a fresh Store for a test.
type namedStore struct {
	name string
	new  func(t *testing.T) Store
}

// testStores returns the in-memory store and, when a test database is
// configured, the CockroachDB one, so a flow test runs against both.
func testStores(t *testing.T) []namedStore {
	t.Helper()
	stores := []namedStore{{"memory", func(*testing.T) Store { return newMemStore() }}}
	if dbtest.Enabled() {
		stores = append(stores, namedStore{"cockroachdb", func(t *testing.T) Store { return testPostgresStore(t) }})
	}
	return stores
}

// testPostgresStore returns a PostgresStore on a brand-new, fully migrated
// database, which is dropped when the test ends.
func testPostgresStore(t *testing.T) *PostgresStore {
	t.Helper()
	t.Helper()
	return NewPostgresStore(testPool(t))
}

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	return dbtest.NewPool(t, "meurpg_identity_test")
}

func TestPostgresStoreUpsertUser(t *testing.T) {
	t.Parallel()
	pool := testPool(t)
	store := NewPostgresStore(pool)
	ctx := t.Context()

	google := ExternalIdentity{Issuer: "https://accounts.google.com", Subject: "1001", Email: "mestre@example.com"}
	first, err := store.UpsertUser(ctx, google)
	if err != nil {
		t.Fatalf("UpsertUser() error = %v", err)
	}

	// Same (issuer, subject): same account, e-mail refreshed or cleared.
	google.Email = "novo@example.com"
	if again, err := store.UpsertUser(ctx, google); err != nil || again != first {
		t.Fatalf("UpsertUser() again = %s, %v; want %s", again, err, first)
	}
	if got := storedEmail(t, pool, google); got != "novo@example.com" {
		t.Errorf("email = %q, want the refreshed one", got)
	}
	google.Email = ""
	if _, err := store.UpsertUser(ctx, google); err != nil {
		t.Fatalf("UpsertUser() error = %v", err)
	}
	if got := storedEmail(t, pool, google); got != "<NULL>" {
		t.Errorf("email = %q, want NULL once the provider stops vouching for it", got)
	}

	// Another subject, or the same subject at another issuer, is someone else.
	for _, other := range []ExternalIdentity{
		{Issuer: "https://accounts.google.com", Subject: "1002"},
		{Issuer: "https://localhost:9443/oauth2/token", Subject: "1001"},
	} {
		id, err := store.UpsertUser(ctx, other)
		if err != nil {
			t.Fatalf("UpsertUser(%v) error = %v", other, err)
		}
		if id == first {
			t.Errorf("UpsertUser(%v) reused account %s", other, first)
		}
	}
	if n := count(t, pool, "users"); n != 3 {
		t.Errorf("users = %d, want 3", n)
	}
}

// TestPostgresStoreUpsertUserRace signs the same new person in from many
// goroutines at once. They must all get the same account, and no stray
// users row may be left behind.
func TestPostgresStoreUpsertUserRace(t *testing.T) {
	t.Parallel()
	dbtest.PoolSize(t, 12) // the racers must overlap: one connection would run them one by one
	pool := testPool(t)
	store := NewPostgresStore(pool)
	id := ExternalIdentity{Issuer: "https://idp.example", Subject: "race"}

	const n = 8
	ids := make([]string, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() { ids[i], errs[i] = store.UpsertUser(t.Context(), id) })
	}
	wg.Wait()

	for i := range n {
		if errs[i] != nil {
			t.Fatalf("UpsertUser() #%d error = %v", i, errs[i])
		}
		if ids[i] != ids[0] {
			t.Errorf("UpsertUser() #%d = %s, want %s like the others", i, ids[i], ids[0])
		}
	}
	if got := count(t, pool, "users"); got != 1 {
		t.Errorf("users = %d, want 1", got)
	}
}

// noIdleLimit is an idleSince so old that no session is idle by it.
var noIdleLimit = time.Unix(0, 0)

func TestPostgresStoreSessions(t *testing.T) {
	t.Parallel()
	pool := testPool(t)
	store := NewPostgresStore(pool)
	ctx := t.Context()

	userID, err := store.UpsertUser(ctx, ExternalIdentity{Issuer: "https://idp.example", Subject: "s1"})
	if err != nil {
		t.Fatalf("UpsertUser() error = %v", err)
	}
	now := time.Now().Truncate(time.Microsecond) // the database's precision
	authTime := now.Add(-time.Hour).Truncate(time.Second)

	newSession := func() ([]byte, Session) {
		_, hash := secret.New()
		s, err := store.createSession(ctx, NewSession{TokenHash: hash, UserID: userID, CreatedAt: now, ExpiresAt: now.Add(SessionLifetime), AuthTime: authTime})
		if err != nil {
			t.Fatalf("CreateSession() error = %v", err)
		}
		return hash, s
	}
	hash1, s1 := newSession()
	hash2, s2 := newSession()

	got, err := store.lookupSession(ctx, hash1, now.Add(time.Minute), noIdleLimit)
	if err != nil {
		t.Fatalf("LookupSession() error = %v", err)
	}
	if got.ID != s1.ID || got.UserID != userID || !got.CreatedAt.Equal(now) || !got.ExpiresAt.Equal(now.Add(SessionLifetime)) {
		t.Errorf("LookupSession() = %+v, want %+v", got, s1)
	}
	var storedAuthTime time.Time
	if err := pool.QueryRow(ctx, "SELECT auth_time FROM auth_sessions WHERE id = $1", s1.ID).Scan(&storedAuthTime); err != nil || !storedAuthTime.Equal(authTime) {
		t.Errorf("auth_time = %v, %v; want %v", storedAuthTime, err, authTime)
	}

	// Only the hash is stored, never the token.
	var storedHash []byte
	if err := pool.QueryRow(ctx, "SELECT token_hash FROM auth_sessions WHERE id = $1", s1.ID).Scan(&storedHash); err != nil || string(storedHash) != string(hash1) {
		t.Errorf("token_hash = %x, %v; want %x", storedHash, err, hash1)
	}

	// Expiry: valid until the last instant before expires_at, never after.
	if _, err := store.lookupSession(ctx, hash1, now.Add(SessionLifetime-time.Microsecond), noIdleLimit); err != nil {
		t.Errorf("LookupSession() just before expiry error = %v", err)
	}
	if _, err := store.lookupSession(ctx, hash1, now.Add(SessionLifetime), noIdleLimit); !errors.Is(err, ErrNotFound) {
		t.Errorf("LookupSession() at expiry error = %v, want ErrNotFound", err)
	}

	// The database refuses a session longer than 30 days.
	_, hash := secret.New()
	if _, err := store.createSession(ctx, NewSession{TokenHash: hash, UserID: userID, CreatedAt: now, ExpiresAt: now.Add(SessionLifetime + time.Second)}); err == nil {
		t.Error("CreateSession() with a 30-day-and-1-second session succeeded, want the CHECK to refuse it")
	}

	// A stream checks its session again by ID (RecheckSession).
	if active, err := store.sessionActive(ctx, s1.ID, now.Add(SessionLifetime-time.Microsecond), noIdleLimit); err != nil || !active {
		t.Errorf("sessionActive() just before expiry = %v, %v; want true", active, err)
	}
	if active, err := store.sessionActive(ctx, s1.ID, now.Add(SessionLifetime), noIdleLimit); err != nil || active {
		t.Errorf("sessionActive() at expiry = %v, %v; want false", active, err)
	}

	// Revoke one, then all.
	if err := store.revokeSession(ctx, s1.ID); err != nil {
		t.Fatalf("RevokeSession() error = %v", err)
	}
	if active, err := store.sessionActive(ctx, s1.ID, now, noIdleLimit); err != nil || active {
		t.Errorf("sessionActive() after revoke = %v, %v; want false", active, err)
	}
	if _, err := store.lookupSession(ctx, hash1, now, noIdleLimit); !errors.Is(err, ErrNotFound) {
		t.Errorf("LookupSession() after revoke error = %v, want ErrNotFound", err)
	}
	if _, err := store.lookupSession(ctx, hash2, now, noIdleLimit); err != nil {
		t.Errorf("the other session was revoked too: %v", err)
	}
	if err := store.revokeUserSessions(ctx, userID); err != nil {
		t.Fatalf("RevokeUserSessions() error = %v", err)
	}
	if _, err := store.lookupSession(ctx, hash2, now, noIdleLimit); !errors.Is(err, ErrNotFound) {
		t.Errorf("LookupSession() after revoke-all error = %v, want ErrNotFound (session %s)", err, s2.ID)
	}

	// Deleting the account deletes its sessions and identities.
	_, s3 := newSession()
	if _, err := pool.Exec(ctx, "DELETE FROM users WHERE id = $1", userID); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	if n := count(t, pool, "auth_sessions"); n != 0 {
		t.Errorf("auth_sessions after deleting the user = %d, want 0 (session %s)", n, s3.ID)
	}
	if n := count(t, pool, "user_identities"); n != 0 {
		t.Errorf("user_identities after deleting the user = %d, want 0", n)
	}
}

// TestPostgresStoreIdleSessions: the idle cutoff, the throttled touch, and
// "sign out of other devices" (migration 00173).
func TestPostgresStoreIdleSessions(t *testing.T) {
	t.Parallel()
	pool := testPool(t)
	store := NewPostgresStore(pool)
	ctx := t.Context()

	userID, err := store.UpsertUser(ctx, ExternalIdentity{Issuer: "https://idp.example", Subject: "idle"})
	if err != nil {
		t.Fatalf("UpsertUser() error = %v", err)
	}
	now := time.Now().Truncate(time.Microsecond)
	newSession := func(user string) ([]byte, Session) {
		_, hash := secret.New()
		s, err := store.createSession(ctx, NewSession{TokenHash: hash, UserID: user, CreatedAt: now, ExpiresAt: now.Add(SessionLifetime)})
		if err != nil {
			t.Fatalf("createSession() error = %v", err)
		}
		return hash, s
	}
	hash, s := newSession(userID)

	// A new session counts as used when it started.
	got, err := store.lookupSession(ctx, hash, now, now.Add(-DefaultSessionIdleTimeout))
	if err != nil || !got.LastUsedAt.Equal(now) {
		t.Fatalf("lookupSession() = %+v, %v; want LastUsedAt %v", got, err, now)
	}

	// Idle: the cutoff is exclusive, like expires_at.
	later := now.Add(DefaultSessionIdleTimeout)
	if _, err := store.lookupSession(ctx, hash, later, later.Add(-DefaultSessionIdleTimeout)); !errors.Is(err, ErrNotFound) {
		t.Errorf("lookupSession() idle for exactly the timeout error = %v, want ErrNotFound", err)
	}
	just := later.Add(-time.Microsecond)
	if _, err := store.lookupSession(ctx, hash, just, just.Add(-DefaultSessionIdleTimeout)); err != nil {
		t.Errorf("lookupSession() just before the timeout error = %v", err)
	}
	if active, err := store.sessionActive(ctx, s.ID, later, later.Add(-DefaultSessionIdleTimeout)); err != nil || active {
		t.Errorf("sessionActive() when idle = %v, %v; want false", active, err)
	}

	// The touch is one conditional UPDATE: it writes only past staleBefore.
	if wrote, err := store.touchSession(ctx, s.ID, now.Add(5*time.Minute), now.Add(-5*time.Minute)); err != nil || wrote {
		t.Errorf("touchSession() inside the window = %v, %v; want no write", wrote, err)
	}
	if wrote, err := store.touchSession(ctx, s.ID, later, later.Add(-sessionTouchEvery)); err != nil || !wrote {
		t.Errorf("touchSession() past the window = %v, %v; want a write", wrote, err)
	}
	if _, err := store.lookupSession(ctx, hash, later, later.Add(-DefaultSessionIdleTimeout)); err != nil {
		t.Errorf("lookupSession() after the touch error = %v, want the session to live", err)
	}
	if wrote, err := store.touchSession(ctx, "00000000-0000-0000-0000-000000000000", later, later); err != nil || wrote {
		t.Errorf("touchSession() of a missing session = %v, %v; want no write", wrote, err)
	}

	// Sign out of other devices: keeps the current one, and counts only
	// sessions that still work, of this user.
	newSession(userID)
	newSession(userID)
	_, idle := newSession(userID)
	if _, err := pool.Exec(ctx, "UPDATE auth_sessions SET last_used_at = $2 WHERE id = $1", idle.ID, now.Add(-time.Hour)); err != nil {
		t.Fatalf("make a session idle: %v", err)
	}
	otherUser, err := store.UpsertUser(ctx, ExternalIdentity{Issuer: "https://idp.example", Subject: "someone-else"})
	if err != nil {
		t.Fatalf("UpsertUser() error = %v", err)
	}
	hashOther, _ := newSession(otherUser)
	check := now.Add(time.Hour)
	idleSince := check.Add(-30 * time.Minute) // "idle" here means unused for 30 minutes
	if n, err := store.countOtherSessions(ctx, userID, s.ID, check, idleSince); err != nil || n != 0 {
		// Every session but the idle one was last used at now, 1 h ago: all idle under this cutoff.
		t.Errorf("countOtherSessions() with a 30-minute cutoff = %d, %v; want 0", n, err)
	}
	idleSince = now.Add(-30 * time.Minute) // only the one set to now-1h is idle
	if n, err := store.countOtherSessions(ctx, userID, s.ID, check, idleSince); err != nil || n != 2 {
		t.Errorf("countOtherSessions() = %d, %v; want 2 (not the current, the idle or another user's)", n, err)
	}
	if n, err := store.revokeOtherSessions(ctx, userID, s.ID, check); err != nil || n != 3 {
		t.Errorf("revokeOtherSessions() = %d, %v; want 3 (the idle one too)", n, err)
	}
	if n, _ := store.countOtherSessions(ctx, userID, s.ID, check, idleSince); n != 0 {
		t.Errorf("countOtherSessions() after = %d, want 0", n)
	}
	if _, err := store.lookupSession(ctx, hash, check, noIdleLimit); err != nil {
		t.Errorf("the current session was revoked too: %v", err)
	}
	if _, err := store.lookupSession(ctx, hashOther, check, noIdleLimit); err != nil {
		t.Errorf("another user's session was revoked: %v", err)
	}
}

func TestPostgresStoreLoginStates(t *testing.T) {
	t.Parallel()
	store := testPostgresStore(t)
	ctx := t.Context()
	now := time.Now().Truncate(time.Microsecond)

	_, hash := secret.New()
	want := LoginState{StateHash: hash, CodeVerifier: "verifier", Nonce: "nonce", ReturnTo: "/campaigns", CreatedAt: now, ExpiresAt: now.Add(loginStateLifetime)}
	if err := store.saveLoginState(ctx, want); err != nil {
		t.Fatalf("SaveLoginState() error = %v", err)
	}
	got, err := store.takeLoginState(ctx, hash, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("TakeLoginState() error = %v", err)
	}
	if got.CodeVerifier != want.CodeVerifier || got.Nonce != want.Nonce || got.ReturnTo != want.ReturnTo ||
		!got.CreatedAt.Equal(want.CreatedAt) || !got.ExpiresAt.Equal(want.ExpiresAt) {
		t.Errorf("TakeLoginState() = %+v, want %+v", got, want)
	}
	// Single use.
	if _, err := store.takeLoginState(ctx, hash, now); !errors.Is(err, ErrNotFound) {
		t.Errorf("second TakeLoginState() error = %v, want ErrNotFound", err)
	}

	// Expired: not returned, and deleted all the same.
	_, hash = secret.New()
	want.StateHash = hash
	if err := store.saveLoginState(ctx, want); err != nil {
		t.Fatalf("SaveLoginState() error = %v", err)
	}
	if _, err := store.takeLoginState(ctx, hash, now.Add(loginStateLifetime)); !errors.Is(err, ErrNotFound) {
		t.Errorf("TakeLoginState() after 10 minutes error = %v, want ErrNotFound", err)
	}
	if n := count(t, store.pool, "oidc_login_states"); n != 0 {
		t.Errorf("oidc_login_states = %d, want 0", n)
	}
}

// TestPostgresStoreLoginStateIntents: a plain sign-in stores NULL in both
// intent columns; an intent stores its kind and data, even when the data is
// empty, and the CHECK refuses what the columns must never hold.
func TestPostgresStoreLoginStateIntents(t *testing.T) {
	t.Parallel()
	store := testPostgresStore(t)
	ctx := t.Context()
	now := time.Now().Truncate(time.Microsecond)
	state := func(kind string, data []byte) LoginState {
		_, hash := secret.New()
		return LoginState{
			StateHash: hash, CodeVerifier: "verifier", Nonce: "nonce", ReturnTo: "/",
			CreatedAt: now, ExpiresAt: now.Add(loginStateLifetime),
			IntentKind: kind, IntentData: data,
		}
	}

	for name, st := range map[string]LoginState{
		"plain sign-in":      state("", nil),
		"intent with data":   state("campaign_invite", []byte{1, 2, 3}),
		"intent, empty data": state("campaign_invite", []byte{}),
		"intent, nil data":   state("campaign_invite", nil),
	} {
		if err := store.saveLoginState(ctx, st); err != nil {
			t.Fatalf("%s: SaveLoginState() error = %v", name, err)
		}
		var kindIsNull, dataIsNull bool
		err := store.pool.QueryRow(ctx,
			"SELECT intent_kind IS NULL, intent_data IS NULL FROM oidc_login_states WHERE state_hash = $1",
			st.StateHash).Scan(&kindIsNull, &dataIsNull)
		if err != nil {
			t.Fatalf("%s: read row: %v", name, err)
		}
		if plain := st.IntentKind == ""; kindIsNull != plain || dataIsNull != plain {
			t.Errorf("%s: intent_kind IS NULL = %v, intent_data IS NULL = %v; want both %v", name, kindIsNull, dataIsNull, plain)
		}

		got, err := store.takeLoginState(ctx, st.StateHash, now)
		if err != nil {
			t.Fatalf("%s: TakeLoginState() error = %v", name, err)
		}
		if got.IntentKind != st.IntentKind || !bytes.Equal(got.IntentData, st.IntentData) {
			t.Errorf("%s: intent = %q %x, want %q %x", name, got.IntentKind, got.IntentData, st.IntentKind, st.IntentData)
		}
		if got.IntentKind != "" && got.IntentData == nil {
			t.Errorf("%s: IntentData is nil for an intent", name)
		}
	}

	// The CHECK is the last line of defense behind the Service's own checks.
	for name, st := range map[string]LoginState{
		"data larger than MaxIntentDataBytes": state("campaign_invite", make([]byte, MaxIntentDataBytes+1)),
		"kind longer than 32 characters":      state(strings.Repeat("k", 33), []byte{1}),
	} {
		if err := store.saveLoginState(ctx, st); err == nil {
			t.Errorf("%s: SaveLoginState() succeeded, want the CHECK to refuse it", name)
		}
	}
	if _, err := store.pool.Exec(ctx,
		"INSERT INTO oidc_login_states (state_hash, code_verifier, nonce, return_to, created_at, expires_at, intent_data) VALUES ($1, 'v', 'n', '/', $2, $3, '\\x01')",
		make([]byte, 32), now, now.Add(time.Minute)); err == nil {
		t.Error("intent_data without intent_kind was accepted")
	}
}

// TestRowLevelTTL checks that CockroachDB deletes expired rows on its own
// (docs/privacy.md promises it).
func TestRowLevelTTL(t *testing.T) {
	t.Parallel()
	pool := testPool(t)

	for table, want := range map[string]string{
		"auth_sessions":     "ttl_expiration_expression = 'expires_at'",
		"oidc_login_states": "ttl_job_cron = '@hourly'",
	} {
		var name, create string
		if err := pool.QueryRow(t.Context(), "SHOW CREATE TABLE "+table).Scan(&name, &create); err != nil {
			t.Fatalf("SHOW CREATE TABLE %s: %v", table, err)
		}
		if !strings.Contains(create, want) || !strings.Contains(create, "ttl = 'on'") {
			t.Errorf("%s is missing row-level TTL (%s): %s", table, want, create)
		}
	}
}

func storedEmail(t *testing.T, pool *pgxpool.Pool, id ExternalIdentity) string {
	t.Helper()
	var email *string
	if err := pool.QueryRow(t.Context(), "SELECT email FROM user_identities WHERE issuer = $1 AND subject = $2", id.Issuer, id.Subject).Scan(&email); err != nil {
		t.Fatalf("read email: %v", err)
	}
	if email == nil {
		return "<NULL>"
	}
	return *email
}

func count(t *testing.T, pool *pgxpool.Pool, table string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM "+table).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

func TestPostgresStoreDisplayNames(t *testing.T) {
	t.Parallel()
	pool := testPool(t)
	store := NewPostgresStore(pool)
	ctx := t.Context()
	userID, err := store.UpsertUser(ctx, ExternalIdentity{Issuer: "https://idp.example", Subject: "dn"})
	if err != nil {
		t.Fatalf("UpsertUser() error = %v", err)
	}

	if err := store.SetDisplayName(ctx, userID, "Pensantus"); err != nil {
		t.Fatalf("SetDisplayName() error = %v", err)
	}
	if got, err := store.DisplayName(ctx, userID); err != nil || got != "Pensantus" {
		t.Errorf("DisplayName() = %q, %v; want Pensantus", got, err)
	}
	// "" is stored as NULL.
	if err := store.SetDisplayName(ctx, userID, ""); err != nil {
		t.Fatalf("SetDisplayName(\"\") error = %v", err)
	}
	var stored *string
	if err := pool.QueryRow(ctx, "SELECT display_name FROM users WHERE id = $1", userID).Scan(&stored); err != nil || stored != nil {
		t.Errorf("display_name = %v, %v; want NULL", stored, err)
	}

	// The CHECK backs the application's 40-character limit.
	if err := store.SetDisplayName(ctx, userID, strings.Repeat("é", MaxDisplayNameLength+1)); err == nil {
		t.Error("SetDisplayName(41 characters) succeeded, want the CHECK to refuse it")
	}

	missing := "6f1c7a52-3b5e-4c55-9d0b-2a51f0c1e0ff"
	if err := store.SetDisplayName(ctx, missing, "X"); !errors.Is(err, ErrNotFound) {
		t.Errorf("SetDisplayName(missing user) error = %v, want ErrNotFound", err)
	}
	if _, err := store.DisplayName(ctx, missing); !errors.Is(err, ErrNotFound) {
		t.Errorf("DisplayName(missing user) error = %v, want ErrNotFound", err)
	}
}

// "Sign out other devices" ends idle sessions too: raising the idle timeout
// later must not make them valid again.
func TestSignOutOthersEndsIdleSessions(t *testing.T) {
	t.Parallel()
	pool := testPool(t)
	store := NewPostgresStore(pool)
	ctx := t.Context()

	userID, err := store.UpsertUser(ctx, ExternalIdentity{Issuer: "https://idp.example", Subject: "idle-sessions"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().Truncate(time.Microsecond)
	mk := func() ([]byte, Session) {
		_, h := secret.New()
		s, err := store.createSession(ctx, NewSession{TokenHash: h, UserID: userID, CreatedAt: now, ExpiresAt: now.Add(SessionLifetime)})
		if err != nil {
			t.Fatal(err)
		}
		return h, s
	}
	_, s1 := mk()
	h2, s2 := mk()
	if _, err := pool.Exec(ctx, "UPDATE auth_sessions SET last_used_at = $2 WHERE id = $1", s2.ID, now.Add(-20*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if n, err := store.revokeOtherSessions(ctx, userID, s1.ID, now); err != nil || n != 1 {
		t.Fatalf("revokeOtherSessions() = %d, %v; want 1", n, err)
	}
	// Operator raises the idle timeout to 30 days.
	if _, err := store.lookupSession(ctx, h2, now, now.Add(-30*24*time.Hour)); !errors.Is(err, ErrNotFound) {
		t.Errorf("signed-out-others session revived after raising idle timeout: err = %v, want ErrNotFound", err)
	}
}
