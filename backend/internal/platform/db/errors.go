package db

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// IsUniqueViolation reports whether err is a unique-index violation (23505)
// of the named index or constraint. CockroachDB names it in the error's
// constraint field.
func IsUniqueViolation(err error, name string) bool {
	pgErr, ok := errors.AsType[*pgconn.PgError](err)
	return ok && pgErr.Code == "23505" && pgErr.ConstraintName == name
}

// IsSerializationFailure reports whether err is a serialization failure
// (40001): the database aborted the statement because it raced another one.
// Outside a transaction, the statement is safe to resend.
func IsSerializationFailure(err error) bool {
	pgErr, ok := errors.AsType[*pgconn.PgError](err)
	return ok && pgErr.Code == sqlstateSerialization
}
