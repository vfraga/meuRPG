package identity

import (
	"errors"
	"testing"
	"time"

	"github.com/PuraFome/meuRPG/backend/internal/platform/secret"
)

// Finding U1-13: "sign out other devices" skips idle-but-unexpired rows, so
// raising the idle timeout later revives them.
func TestReview1_SignOutOthersIdleRevived(t *testing.T) {
	t.Parallel()
	pool := testPool(t)
	store := NewPostgresStore(pool)
	ctx := t.Context()

	userID, err := store.UpsertUser(ctx, ExternalIdentity{Issuer: "https://idp.example", Subject: "r1u113"})
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
	idle14 := 14 * 24 * time.Hour
	if _, err := store.revokeOtherSessions(ctx, userID, s1.ID, now, now.Add(-idle14)); err != nil {
		t.Fatal(err)
	}
	// Operator raises the idle timeout to 30 days.
	if _, err := store.lookupSession(ctx, h2, now, now.Add(-30*24*time.Hour)); !errors.Is(err, ErrNotFound) {
		t.Errorf("signed-out-others session revived after raising idle timeout: err = %v, want ErrNotFound", err)
	}
}
