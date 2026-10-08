package identity

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	identityv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/identity/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/logging"
	"github.com/PuraFome/meuRPG/backend/internal/platform/names"
)

// MaxDisplayNameLength is the longest display name, in characters
// (ADR-0009). The users_display_name_length CHECK says the same.
const MaxDisplayNameLength = 40

// sessionKey is the context key for the current Session. It is unexported,
// and only authenticate (below) sets it, after looking the session cookie
// up in the database. No exported function puts a session into a context:
// other modules learn who is calling through UserID (the authz.Caller
// interface), and their tests pass a fake of that interface instead
// (docs/architecture.md#who-is-calling).
type sessionKey struct{}

// sessionFromContext returns the session that Interceptor found for this
// request, if any.
func sessionFromContext(ctx context.Context) (Session, bool) {
	s, ok := ctx.Value(sessionKey{}).(Session)
	return s, ok
}

// requireSession returns the caller's session, or an `unauthenticated`
// Connect error to return as is. It fails closed: a handler whose service
// was mounted without Interceptor always gets the error.
func requireSession(ctx context.Context) (Session, error) {
	if s, ok := sessionFromContext(ctx); ok {
		return s, nil
	}
	return Session{}, errUnauthenticated()
}

// UserID returns the ID of the user whose session Interceptor found for
// this request, or an `unauthenticated` Connect error to return as is. It
// implements authz.Caller, which is how other modules learn who is calling:
// they mount Interceptor on their Connect service and call UserID (through
// package authz) in their handlers. Without Interceptor, it always fails.
func (s *Service) UserID(ctx context.Context) (string, error) {
	session, err := requireSession(ctx)
	if err != nil {
		return "", err
	}
	return session.UserID, nil
}

// RecheckSession reads the session that Interceptor found for this request
// again, by its ID, and returns an `unauthenticated` Connect error, to
// return as is, if it ended since: the user signed out, the session was
// revoked, or it expired. Other errors mean the store could not answer. It
// implements authz.SessionRechecker, for long-lived streams
// (authz.RecheckCampaignMember), which would otherwise keep the session
// they started with.
func (s *Service) RecheckSession(ctx context.Context) error {
	session, err := requireSession(ctx)
	if err != nil {
		return err
	}
	now := s.now()
	active, err := s.store.sessionActive(ctx, session.ID, now, now.Add(-s.idleTimeout))
	if err != nil {
		return err
	}
	if !active {
		return errUnauthenticated()
	}
	// An open stream is a use of the session: without this, someone who
	// watches a live session for days without any other request would hit
	// the idle timeout. The session in ctx was read when the stream began,
	// so its LastUsedAt is old; the conditional UPDATE still writes at most
	// once per interval.
	s.touchSession(ctx, session)
	return nil
}

// errUnauthenticated is the error for a request without a valid session.
func errUnauthenticated() error {
	err := connect.NewError(connect.CodeUnauthenticated, errors.New("sign in to continue"))
	err.Meta().Set("Cache-Control", "no-store")
	return err
}

// Interceptor returns a Connect interceptor that reads the session cookie,
// looks the session up and, when it is valid, puts it in the context for
// UserID (and this package's own handlers). It is the only place a session
// ever enters a context.
//
// A missing or invalid session is not an error here: some RPCs work signed
// out, so each handler decides, with UserID or requireSession. A database failure is
// an error (`unavailable`), because answering `unauthenticated` would make
// the app think the user signed out.
func (s *Service) Interceptor() connect.Interceptor {
	return &sessionInterceptor{service: s}
}

type sessionInterceptor struct {
	service *Service
}

// WrapUnary implements connect.Interceptor.
func (i *sessionInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		if req.Spec().IsClient {
			return next(ctx, req)
		}
		ctx, err := i.service.authenticate(ctx, req.Header())
		if err != nil {
			return nil, err
		}
		return next(ctx, req)
	}
}

// WrapStreamingClient implements connect.Interceptor; clients pass through.
func (i *sessionInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

// WrapStreamingHandler implements connect.Interceptor.
func (i *sessionInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		ctx, err := i.service.authenticate(ctx, conn.RequestHeader())
		if err != nil {
			return err
		}
		return next(ctx, conn)
	}
}

// AuthenticateRequest is Interceptor for plain HTTP routes, such as the
// image upload and download (package maps): it returns r's context with
// the request's session in it, when the session cookie holds a valid one,
// so UserID works in the handler. Like Interceptor, it only ever trusts the
// cookie, checked against the database: it cannot put anyone else's
// session in a context.
//
// A missing or invalid session is not an error; each handler decides, with
// UserID. The error, an `unavailable` Connect error, means the database did
// not answer.
func (s *Service) AuthenticateRequest(r *http.Request) (context.Context, error) {
	return s.authenticate(r.Context(), r.Header)
}

// authenticate adds the request's session to ctx, when there is a valid one.
func (s *Service) authenticate(ctx context.Context, header http.Header) (context.Context, error) {
	token, ok := cookieValue(header, SessionCookieName)
	if !ok {
		return ctx, nil
	}
	session, err := s.lookupSession(ctx, token)
	switch {
	case err == nil:
		// The user id (an account UUID, pseudonymous) joins every log line of
		// this request.
		logging.SetUserID(ctx, session.UserID)
		s.touchSession(ctx, session)
		return context.WithValue(ctx, sessionKey{}, session), nil
	case errors.Is(err, errNoSession):
		return ctx, nil
	default:
		s.logger.ErrorContext(ctx, "cannot look up the session", "error", err)
		return ctx, connect.NewError(connect.CodeUnavailable, errors.New("cannot check the session right now, please try again"))
	}
}

// GetMe implements identityv1connect.IdentityServiceHandler.
func (s *Service) GetMe(
	ctx context.Context,
	_ *connect.Request[identityv1.GetMeRequest],
) (*connect.Response[identityv1.GetMeResponse], error) {
	session, err := requireSession(ctx)
	if err != nil {
		return nil, err
	}
	displayName, err := s.store.DisplayName(ctx, session.UserID)
	if err != nil {
		return nil, s.storeError(ctx, "cannot read the display name", err)
	}
	res := connect.NewResponse(&identityv1.GetMeResponse{
		User:             &identityv1.User{Id: session.UserID, DisplayName: displayName},
		SessionExpiresAt: timestamppb.New(session.ExpiresAt),
	})
	res.Header().Set("Cache-Control", "no-store")
	return res, nil
}

// UpdateProfile implements identityv1connect.IdentityServiceHandler.
func (s *Service) UpdateProfile(
	ctx context.Context,
	req *connect.Request[identityv1.UpdateProfileRequest],
) (*connect.Response[identityv1.UpdateProfileResponse], error) {
	session, err := requireSession(ctx)
	if err != nil {
		return nil, err
	}

	// An empty (or all-spaces) name removes it; anything else must be a
	// valid name.
	displayName, err := names.Clean(req.Msg.GetDisplayName(), MaxDisplayNameLength)
	switch {
	case errors.Is(err, names.ErrEmpty):
		displayName = ""
	case err != nil:
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("display_name %w", err))
	}

	if err := s.store.SetDisplayName(ctx, session.UserID, displayName); err != nil {
		return nil, s.storeError(ctx, "cannot save the display name", err)
	}
	res := connect.NewResponse(&identityv1.UpdateProfileResponse{
		User: &identityv1.User{Id: session.UserID, DisplayName: displayName},
	})
	res.Header().Set("Cache-Control", "no-store")
	return res, nil
}

// storeError logs a failed Store call and turns it into the Connect error
// the client gets. The account of a valid session always exists (deleting
// it deletes its sessions), so ErrNotFound means it was deleted during this
// very request: the caller is signed out.
func (s *Service) storeError(ctx context.Context, msg string, err error) error {
	if errors.Is(err, ErrNotFound) {
		return errUnauthenticated()
	}
	s.logger.ErrorContext(ctx, msg, "error", err) //nolint:sloglint // each call site passes a fixed message
	return connect.NewError(connect.CodeUnavailable, errors.New("cannot reach the database right now, please try again"))
}

// SignOut implements identityv1connect.IdentityServiceHandler. It deletes
// the session row, so the token stops working at once on every server
// instance, and clears the cookie.
func (s *Service) SignOut(
	ctx context.Context,
	_ *connect.Request[identityv1.SignOutRequest],
) (*connect.Response[identityv1.SignOutResponse], error) {
	clearCookie := expiredCookie(SessionCookieName).String()

	session, err := requireSession(ctx)
	if err != nil {
		// Also clear a stale cookie (expired or already revoked), so the
		// browser stops sending it.
		if connectErr, ok := errors.AsType[*connect.Error](err); ok {
			connectErr.Meta().Add("Set-Cookie", clearCookie)
		}
		return nil, err
	}
	if err := s.store.revokeSession(ctx, session.ID); err != nil {
		s.logger.ErrorContext(ctx, "cannot revoke the session", "error", err)
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("cannot sign out right now, please try again"))
	}

	res := connect.NewResponse(&identityv1.SignOutResponse{})
	res.Header().Add("Set-Cookie", clearCookie)
	res.Header().Set("Cache-Control", "no-store")
	return res, nil
}

// clampInt32 narrows a row count to the response's int32; a user never has
// billions of sessions, so the clamp only keeps the conversion honest.
func clampInt32(n int64) int32 {
	return int32(min(n, math.MaxInt32)) //nolint:gosec // G115: clamped on the left
}

// SignOutOtherSessions implements identityv1connect.IdentityServiceHandler.
// It deletes the user's other session rows in one statement, so those tokens
// stop working at once on every instance. The current session and its cookie
// are untouched.
func (s *Service) SignOutOtherSessions(
	ctx context.Context,
	_ *connect.Request[identityv1.SignOutOtherSessionsRequest],
) (*connect.Response[identityv1.SignOutOtherSessionsResponse], error) {
	session, err := requireSession(ctx)
	if err != nil {
		return nil, err
	}
	now := s.now()
	ended, err := s.store.revokeOtherSessions(ctx, session.UserID, session.ID, now)
	if err != nil {
		s.logger.ErrorContext(ctx, "cannot revoke the other sessions", "error", err)
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("cannot sign out the other devices right now, please try again"))
	}
	// Counts in the audit trail, never tokens or hashes.
	s.logger.InfoContext(ctx, "signed out of other devices", "ended", ended)
	res := connect.NewResponse(&identityv1.SignOutOtherSessionsResponse{EndedCount: clampInt32(ended)})
	res.Header().Set("Cache-Control", "no-store")
	return res, nil
}

// CountOtherSessions implements identityv1connect.IdentityServiceHandler.
func (s *Service) CountOtherSessions(
	ctx context.Context,
	_ *connect.Request[identityv1.CountOtherSessionsRequest],
) (*connect.Response[identityv1.CountOtherSessionsResponse], error) {
	session, err := requireSession(ctx)
	if err != nil {
		return nil, err
	}
	now := s.now()
	n, err := s.store.countOtherSessions(ctx, session.UserID, session.ID, now, now.Add(-s.idleTimeout))
	if err != nil {
		return nil, s.storeError(ctx, "cannot count the other sessions", err)
	}
	res := connect.NewResponse(&identityv1.CountOtherSessionsResponse{OtherSessions: clampInt32(n)})
	res.Header().Set("Cache-Control", "no-store")
	return res, nil
}

// disabledService answers every IdentityService RPC with `unavailable`, for
// a server where sign-in is not configured (see MountDisabled).
type disabledService struct{}

// GetMe implements identityv1connect.IdentityServiceHandler.
func (disabledService) GetMe(
	context.Context,
	*connect.Request[identityv1.GetMeRequest],
) (*connect.Response[identityv1.GetMeResponse], error) {
	return nil, connect.NewError(connect.CodeUnavailable, errDisabled)
}

// UpdateProfile implements identityv1connect.IdentityServiceHandler.
func (disabledService) UpdateProfile(
	context.Context,
	*connect.Request[identityv1.UpdateProfileRequest],
) (*connect.Response[identityv1.UpdateProfileResponse], error) {
	return nil, connect.NewError(connect.CodeUnavailable, errDisabled)
}

// SignOut implements identityv1connect.IdentityServiceHandler.
func (disabledService) SignOut(
	context.Context,
	*connect.Request[identityv1.SignOutRequest],
) (*connect.Response[identityv1.SignOutResponse], error) {
	return nil, connect.NewError(connect.CodeUnavailable, errDisabled)
}

// SignOutOtherSessions implements identityv1connect.IdentityServiceHandler.
func (disabledService) SignOutOtherSessions(
	context.Context,
	*connect.Request[identityv1.SignOutOtherSessionsRequest],
) (*connect.Response[identityv1.SignOutOtherSessionsResponse], error) {
	return nil, connect.NewError(connect.CodeUnavailable, errDisabled)
}

// CountOtherSessions implements identityv1connect.IdentityServiceHandler.
func (disabledService) CountOtherSessions(
	context.Context,
	*connect.Request[identityv1.CountOtherSessionsRequest],
) (*connect.Response[identityv1.CountOtherSessionsResponse], error) {
	return nil, connect.NewError(connect.CodeUnavailable, errDisabled)
}
