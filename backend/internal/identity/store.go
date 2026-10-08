package identity

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/PuraFome/meuRPG/backend/internal/identity/identitydb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
)

// ErrNotFound is returned by a Store when the row does not exist or has
// expired.
var ErrNotFound = errors.New("identity: not found")

// Store persists what the identity module needs. PostgresStore is the real
// one; tests use an in-memory fake.
//
// The methods for sessions and login states are unexported on purpose: only
// this package can create or look up a session, and only this package can
// implement a Store. Other modules get accounts and display names, but they
// cannot forge a session, directly or through a fake Store
// (docs/architecture.md#who-is-calling).
type Store interface {
	// saveLoginState records a sign-in that has just started.
	saveLoginState(ctx context.Context, state LoginState) error
	// takeLoginState deletes and returns the login state with this hash, so
	// it can be used only once. It returns ErrNotFound when there is no such
	// state or when it expired before now.
	takeLoginState(ctx context.Context, stateHash []byte, now time.Time) (LoginState, error)

	// UpsertUser returns the account linked to the identity, creating both
	// on the first sign-in, and refreshes the stored e-mail.
	UpsertUser(ctx context.Context, identity ExternalIdentity) (userID string, err error)

	// createSession stores a new session.
	createSession(ctx context.Context, session NewSession) (Session, error)
	// lookupSession returns the session with this token hash, or
	// ErrNotFound if there is none, it expired before now, or it has not
	// been used since idleSince (the idle timeout).
	lookupSession(ctx context.Context, tokenHash []byte, now, idleSince time.Time) (Session, error)
	// sessionActive reports whether the session with this ID still exists,
	// has not expired before now and has been used since idleSince.
	sessionActive(ctx context.Context, sessionID string, now, idleSince time.Time) (bool, error)
	// touchSession records that the session was used at now, but only when
	// its stored last use is older than staleBefore (one conditional
	// UPDATE), so a busy session is not a write per request. It reports
	// whether it wrote.
	touchSession(ctx context.Context, sessionID string, now, staleBefore time.Time) (bool, error)
	// revokeSession deletes one session. Deleting a missing one is not an
	// error.
	revokeSession(ctx context.Context, sessionID string) error
	// revokeUserSessions deletes every session of a user: "sign out
	// everywhere", or a suspected account takeover.
	revokeUserSessions(ctx context.Context, userID string) error
	// revokeOtherSessions deletes the user's sessions that have not expired
	// at now, idle ones included, except keepID, and returns how many it
	// deleted: "sign out of other devices".
	revokeOtherSessions(ctx context.Context, userID, keepID string, now time.Time) (int64, error)
	// countOtherSessions counts the other sessions that still work (not
	// expired at now, used since idleSince).
	countOtherSessions(ctx context.Context, userID, keepID string, now, idleSince time.Time) (int64, error)

	// DisplayName returns the user's display name, or "" if they have not
	// set one. It returns ErrNotFound if the user does not exist.
	DisplayName(ctx context.Context, userID string) (string, error)
	// SetDisplayName saves the user's display name; "" removes it. It
	// returns ErrNotFound if the user does not exist.
	SetDisplayName(ctx context.Context, userID, displayName string) error
	// DisplayNames returns the display names of the given users, keyed by
	// user ID. Users without one, or that do not exist, are left out.
	DisplayNames(ctx context.Context, userIDs []string) (map[string]string, error)
}

// LoginState is a sign-in in progress (table oidc_login_states).
type LoginState struct {
	StateHash    []byte
	CodeVerifier string
	Nonce        string
	ReturnTo     string
	CreatedAt    time.Time
	ExpiresAt    time.Time

	// IntentKind names the intent to complete after signing in (see
	// IntentHandler), or is empty for a plain sign-in. IntentData is what
	// that intent's Prepare returned; never nil when IntentKind is set.
	IntentKind string
	IntentData []byte
}

// ExternalIdentity is a verified identity at an OpenID Connect provider.
type ExternalIdentity struct {
	Issuer  string
	Subject string
	// Email is a verified e-mail, or empty. An empty value clears the one
	// stored before, so the column always mirrors the latest sign-in.
	Email string
}

// NewSession is a session about to be created.
type NewSession struct {
	TokenHash []byte
	UserID    string
	CreatedAt time.Time
	ExpiresAt time.Time
	// AuthTime is the provider's auth_time; zero when unknown.
	AuthTime time.Time
}

// PostgresStore is the Store backed by CockroachDB. The SQL is in
// queries.sql; sqlc generates the Go methods in package identitydb.
//
// Statements that stand alone run without an explicit transaction:
// CockroachDB retries a single-statement (implicit) transaction by itself
// when it hits a conflict. Only UpsertUser, which needs several statements
// to agree, goes through db.InTx.
type PostgresStore struct {
	pool    *pgxpool.Pool
	queries *identitydb.Queries
}

// NewPostgresStore returns a Store that uses pool.
func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool, queries: identitydb.New(pool)}
}

var _ Store = (*PostgresStore)(nil)

// saveLoginState implements Store.
func (s *PostgresStore) saveLoginState(ctx context.Context, st LoginState) error {
	// A plain sign-in stores NULL in both intent columns; an intent stores
	// both, even with empty data (pgx sends a nil slice as NULL).
	var intentKind *string
	var intentData []byte
	if st.IntentKind != "" {
		intentKind = &st.IntentKind
		intentData = st.IntentData
		if intentData == nil {
			intentData = []byte{}
		}
	}
	err := s.queries.InsertLoginState(ctx, identitydb.InsertLoginStateParams{
		StateHash:    st.StateHash,
		CodeVerifier: st.CodeVerifier,
		Nonce:        st.Nonce,
		ReturnTo:     st.ReturnTo,
		CreatedAt:    st.CreatedAt,
		ExpiresAt:    st.ExpiresAt,
		IntentKind:   intentKind,
		IntentData:   intentData,
	})
	if err != nil {
		return fmt.Errorf("insert login state: %w", err)
	}
	return nil
}

// takeLoginState implements Store. DELETE ... RETURNING reads and removes
// the row in one statement, so two callbacks racing with the same state
// cannot both get it.
func (s *PostgresStore) takeLoginState(ctx context.Context, stateHash []byte, now time.Time) (LoginState, error) {
	row, err := s.queries.TakeLoginState(ctx, stateHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return LoginState{}, ErrNotFound
	}
	if err != nil {
		return LoginState{}, fmt.Errorf("take login state: %w", err)
	}
	// Expired rows linger until the TTL job runs; they are deleted above
	// all the same.
	if !now.Before(row.ExpiresAt) {
		return LoginState{}, ErrNotFound
	}
	st := LoginState{
		StateHash:    stateHash,
		CodeVerifier: row.CodeVerifier,
		Nonce:        row.Nonce,
		ReturnTo:     row.ReturnTo,
		CreatedAt:    row.CreatedAt,
		ExpiresAt:    row.ExpiresAt,
	}
	if row.IntentKind != nil {
		st.IntentKind = *row.IntentKind
		st.IntentData = row.IntentData
		if st.IntentData == nil {
			st.IntentData = []byte{}
		}
	}
	return st, nil
}

// UpsertUser implements Store.
//
// Two first sign-ins of the same person at the same moment both see "no
// identity yet" and both try to create one. CockroachDB aborts one of them
// with a serialization error (40001); InTx runs it again, and the second
// attempt finds the other's row, with no stray users row left behind
// (TestPostgresStoreUpsertUserRace).
func (s *PostgresStore) UpsertUser(ctx context.Context, id ExternalIdentity) (string, error) {
	var email *string
	if id.Email != "" {
		email = &id.Email
	}

	var userID string
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)

		// Returning user: refresh the e-mail and we are done. This runs on
		// every sign-in, so a changed or unverified e-mail is updated or
		// cleared right away.
		var err error
		userID, err = q.UpdateIdentityEmail(ctx, identitydb.UpdateIdentityEmailParams{
			Issuer:  id.Issuer,
			Subject: id.Subject,
			Email:   email,
		})
		if err == nil {
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("update identity: %w", err)
		}

		// First sign-in: create the account and link the identity to it.
		if userID, err = q.InsertUser(ctx); err != nil {
			return fmt.Errorf("insert user: %w", err)
		}
		err = q.InsertIdentity(ctx, identitydb.InsertIdentityParams{
			Issuer:  id.Issuer,
			Subject: id.Subject,
			UserID:  userID,
			Email:   email,
		})
		if err != nil {
			return fmt.Errorf("insert identity: %w", err)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return userID, nil
}

// createSession implements Store.
func (s *PostgresStore) createSession(ctx context.Context, ns NewSession) (Session, error) {
	var authTime *time.Time
	if !ns.AuthTime.IsZero() {
		authTime = &ns.AuthTime
	}
	id, err := s.queries.InsertSession(ctx, identitydb.InsertSessionParams{
		TokenHash: ns.TokenHash,
		UserID:    ns.UserID,
		CreatedAt: ns.CreatedAt,
		ExpiresAt: ns.ExpiresAt,
		AuthTime:  authTime,
	})
	if err != nil {
		return Session{}, fmt.Errorf("insert session: %w", err)
	}
	return Session{ID: id, UserID: ns.UserID, CreatedAt: ns.CreatedAt, ExpiresAt: ns.ExpiresAt}, nil
}

// lookupSession implements Store.
func (s *PostgresStore) lookupSession(ctx context.Context, tokenHash []byte, now, idleSince time.Time) (Session, error) {
	row, err := s.queries.LookupSession(ctx, identitydb.LookupSessionParams{TokenHash: tokenHash, Now: now, IdleSince: idleSince})
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, ErrNotFound
	}
	if err != nil {
		return Session{}, fmt.Errorf("look up session: %w", err)
	}
	return Session{ID: row.ID, UserID: row.UserID, CreatedAt: row.CreatedAt, ExpiresAt: row.ExpiresAt, LastUsedAt: row.LastUsedAt}, nil
}

// sessionActive implements Store.
func (s *PostgresStore) sessionActive(ctx context.Context, sessionID string, now, idleSince time.Time) (bool, error) {
	active, err := s.queries.SessionIsActive(ctx, identitydb.SessionIsActiveParams{ID: sessionID, Now: now, IdleSince: idleSince})
	if err != nil {
		return false, fmt.Errorf("check session: %w", err)
	}
	return active, nil
}

// touchSession implements Store.
func (s *PostgresStore) touchSession(ctx context.Context, sessionID string, now, staleBefore time.Time) (bool, error) {
	n, err := s.queries.TouchSession(ctx, identitydb.TouchSessionParams{ID: sessionID, Now: now, StaleBefore: staleBefore})
	if err != nil {
		return false, fmt.Errorf("touch session: %w", err)
	}
	return n > 0, nil
}

// revokeSession implements Store.
func (s *PostgresStore) revokeSession(ctx context.Context, sessionID string) error {
	if err := s.queries.DeleteSession(ctx, sessionID); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

// revokeUserSessions implements Store.
func (s *PostgresStore) revokeUserSessions(ctx context.Context, userID string) error {
	if err := s.queries.DeleteUserSessions(ctx, userID); err != nil {
		return fmt.Errorf("delete user sessions: %w", err)
	}
	return nil
}

// revokeOtherSessions implements Store.
func (s *PostgresStore) revokeOtherSessions(ctx context.Context, userID, keepID string, now time.Time) (int64, error) {
	n, err := s.queries.DeleteOtherUserSessions(ctx, identitydb.DeleteOtherUserSessionsParams{UserID: userID, KeepID: keepID, Now: now})
	if err != nil {
		return 0, fmt.Errorf("delete other sessions: %w", err)
	}
	return n, nil
}

// countOtherSessions implements Store.
func (s *PostgresStore) countOtherSessions(ctx context.Context, userID, keepID string, now, idleSince time.Time) (int64, error) {
	n, err := s.queries.CountOtherSessions(ctx, identitydb.CountOtherSessionsParams{UserID: userID, KeepID: keepID, Now: now, IdleSince: idleSince})
	if err != nil {
		return 0, fmt.Errorf("count other sessions: %w", err)
	}
	return n, nil
}

// DisplayName implements Store.
func (s *PostgresStore) DisplayName(ctx context.Context, userID string) (string, error) {
	name, err := s.queries.GetDisplayName(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("get display name: %w", err)
	}
	if name == nil {
		return "", nil
	}
	return *name, nil
}

// SetDisplayName implements Store.
func (s *PostgresStore) SetDisplayName(ctx context.Context, userID, displayName string) error {
	var name *string // NULL removes the name
	if displayName != "" {
		name = &displayName
	}
	n, err := s.queries.SetDisplayName(ctx, identitydb.SetDisplayNameParams{ID: userID, DisplayName: name})
	if err != nil {
		return fmt.Errorf("set display name: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// VerifiedEmails returns the verified e-mail addresses of a user, as the
// provider gave them (the identity module's public answer to "who is this?"
// for campaigns.Profiles). It is empty for a user whose e-mail the provider
// did not verify: such an e-mail is never stored (docs/privacy.md).
func (s *PostgresStore) VerifiedEmails(ctx context.Context, userID string) ([]string, error) {
	emails, err := s.queries.ListVerifiedEmails(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list verified emails: %w", err)
	}
	return emails, nil
}

// DisplayNames implements Store. It is also the identity module's public
// answer to "what do we call these people?" (campaigns.Profiles), so that
// other modules never read the users table themselves.
func (s *PostgresStore) DisplayNames(ctx context.Context, userIDs []string) (map[string]string, error) {
	rows, err := s.queries.ListDisplayNames(ctx, userIDs)
	if err != nil {
		return nil, fmt.Errorf("list display names: %w", err)
	}
	names := make(map[string]string, len(rows))
	for _, row := range rows {
		names[row.ID] = row.DisplayName
	}
	return names, nil
}
