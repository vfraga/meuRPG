package identity

import (
	"context"
	"crypto/rand"
	"fmt"
	"sync"
	"time"
)

// memStore is an in-memory Store for tests that run without a database. It
// mirrors PostgresStore's rules (single-use login states, expiry filters);
// store_integration_test.go checks the real one against the same rules.
type memStore struct {
	mu          sync.Mutex
	loginStates map[string]LoginState // key: string(state hash)
	identities  map[[2]string]memIdentity
	users       map[string]string     // user ID -> display name
	sessions    map[string]memSession // key: session ID
	touches     int                   // successful touchSession writes
}

type memIdentity struct {
	userID string
	email  string
}

type memSession struct {
	Session
	tokenHash string
	authTime  time.Time
}

func newMemStore() *memStore {
	return &memStore{
		loginStates: map[string]LoginState{},
		identities:  map[[2]string]memIdentity{},
		users:       map[string]string{},
		sessions:    map[string]memSession{},
	}
}

var _ Store = (*memStore)(nil)

func (m *memStore) saveLoginState(_ context.Context, st LoginState) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.loginStates[string(st.StateHash)] = st
	return nil
}

func (m *memStore) takeLoginState(_ context.Context, stateHash []byte, now time.Time) (LoginState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	st, ok := m.loginStates[string(stateHash)]
	delete(m.loginStates, string(stateHash))
	if !ok || !now.Before(st.ExpiresAt) {
		return LoginState{}, ErrNotFound
	}
	return st, nil
}

func (m *memStore) UpsertUser(_ context.Context, id ExternalIdentity) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := [2]string{id.Issuer, id.Subject}
	existing, ok := m.identities[key]
	if !ok {
		existing.userID = rand.Text()
		m.users[existing.userID] = ""
	}
	existing.email = id.Email
	m.identities[key] = existing
	return existing.userID, nil
}

func (m *memStore) createSession(_ context.Context, ns NewSession) (Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.users[ns.UserID]; !ok {
		return Session{}, fmt.Errorf("user %s does not exist", ns.UserID)
	}
	if !ns.ExpiresAt.After(ns.CreatedAt) || ns.ExpiresAt.Sub(ns.CreatedAt) > SessionLifetime {
		return Session{}, fmt.Errorf("session lifetime out of range") // the database CHECK
	}
	s := Session{ID: rand.Text(), UserID: ns.UserID, CreatedAt: ns.CreatedAt, ExpiresAt: ns.ExpiresAt, LastUsedAt: ns.CreatedAt}
	m.sessions[s.ID] = memSession{Session: s, tokenHash: string(ns.TokenHash), authTime: ns.AuthTime}
	return s, nil
}

func (m *memStore) lookupSession(_ context.Context, tokenHash []byte, now, idleSince time.Time) (Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range m.sessions {
		if s.tokenHash == string(tokenHash) && s.ExpiresAt.After(now) && s.LastUsedAt.After(idleSince) {
			return s.Session, nil
		}
	}
	return Session{}, ErrNotFound
}

func (m *memStore) sessionActive(_ context.Context, sessionID string, now, idleSince time.Time) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[sessionID]
	return ok && s.ExpiresAt.After(now) && s.LastUsedAt.After(idleSince), nil
}

// touches counts the writes touchSession made, for the throttle tests.
func (m *memStore) touchCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.touches
}

func (m *memStore) touchSession(_ context.Context, sessionID string, now, staleBefore time.Time) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[sessionID]
	if !ok || !s.LastUsedAt.Before(staleBefore) {
		return false, nil
	}
	s.LastUsedAt = now
	m.sessions[sessionID] = s
	m.touches++
	return true, nil
}

func (m *memStore) revokeOtherSessions(_ context.Context, userID, keepID string, now time.Time) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var n int64
	for id, s := range m.sessions {
		if s.UserID == userID && id != keepID && s.ExpiresAt.After(now) {
			delete(m.sessions, id)
			n++
		}
	}
	return n, nil
}

func (m *memStore) countOtherSessions(_ context.Context, userID, keepID string, now, idleSince time.Time) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var n int64
	for id, s := range m.sessions {
		if s.UserID == userID && id != keepID && s.ExpiresAt.After(now) && s.LastUsedAt.After(idleSince) {
			n++
		}
	}
	return n, nil
}

func (m *memStore) revokeSession(_ context.Context, sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, sessionID)
	return nil
}

func (m *memStore) revokeUserSessions(_ context.Context, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, s := range m.sessions {
		if s.UserID == userID {
			delete(m.sessions, id)
		}
	}
	return nil
}

func (m *memStore) DisplayName(_ context.Context, userID string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	name, ok := m.users[userID]
	if !ok {
		return "", ErrNotFound
	}
	return name, nil
}

func (m *memStore) SetDisplayName(_ context.Context, userID, displayName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.users[userID]; !ok {
		return ErrNotFound
	}
	m.users[userID] = displayName
	return nil
}

func (m *memStore) DisplayNames(_ context.Context, userIDs []string) (map[string]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	names := map[string]string{}
	for _, id := range userIDs {
		if name := m.users[id]; name != "" {
			names[id] = name
		}
	}
	return names, nil
}

// tamperLoginStates changes every stored login state, e.g. to swap the PKCE
// verifier behind the Service's back.
func (m *memStore) tamperLoginStates(edit func(*LoginState)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, st := range m.loginStates {
		edit(&st)
		m.loginStates[k] = st
	}
}

func (m *memStore) sessionCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sessions)
}

func (m *memStore) identity(issuer, subject string) (memIdentity, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.identities[[2]string{issuer, subject}]
	return id, ok
}

func (m *memStore) sessionAuthTime(sessionID string) time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sessions[sessionID].authTime
}

// failingStore is a Store whose every method fails, to test how the Service
// behaves when the database is down.
type failingStore struct{ Store }

var errStoreDown = fmt.Errorf("database is down")

func (failingStore) saveLoginState(context.Context, LoginState) error { return errStoreDown }
func (failingStore) lookupSession(context.Context, []byte, time.Time, time.Time) (Session, error) {
	return Session{}, errStoreDown
}
