package identity

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
)

// r1u112Store fails createSession while failCreate is set.
type r1u112Store struct {
	*memStore
	failCreate atomic.Bool
}

func (s *r1u112Store) createSession(ctx context.Context, ns NewSession) (Session, error) {
	if s.failCreate.Load() {
		return Session{}, errors.New("simulated DB blip")
	}
	return s.memStore.createSession(ctx, ns)
}

// Finding U1-12: completeLogin revokes the old session before the new one is
// created; a createSession failure leaves a signed-in user with no session.
func TestReview1_OldSessionSurvivesFailedRelogin(t *testing.T) {
	t.Parallel()
	st := &r1u112Store{memStore: newMemStore()}
	h := newHarness(t, withStore(st))
	old := h.signIn()
	if st.memStore.sessionCount() != 1 {
		t.Fatalf("sessions = %d, want 1", st.memStore.sessionCount())
	}

	st.failCreate.Store(true)
	callbackURL, loginCookie := h.beginLogin("/")
	rec := h.finishLogin(callbackURL, loginCookie, old)
	if rec.Code == http.StatusSeeOther {
		t.Fatalf("expected the relogin to fail, got 303")
	}
	st.failCreate.Store(false)

	resp, err := h.getMe(old)
	if err != nil {
		t.Fatalf("old session no longer valid after failed re-login (user logged out): %v", err)
	}
	_ = resp
}
