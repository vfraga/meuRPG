package db

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestIsUniqueViolation(t *testing.T) {
	t.Parallel()
	unique := &pgconn.PgError{Code: "23505", ConstraintName: "idx_a"}
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"same index", unique, true},
		{"wrapped", fmt.Errorf("insert: %w", unique), true},
		{"other index", &pgconn.PgError{Code: "23505", ConstraintName: "idx_b"}, false},
		{"other code", &pgconn.PgError{Code: "23503", ConstraintName: "idx_a"}, false},
		{"not a database error", errors.New("boom"), false},
		{"nil", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := IsUniqueViolation(tt.err, "idx_a"); got != tt.want {
				t.Errorf("IsUniqueViolation() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsSerializationFailure(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"40001", &pgconn.PgError{Code: "40001"}, true},
		{"wrapped", fmt.Errorf("take: %w", &pgconn.PgError{Code: "40001"}), true},
		{"deadlock is not it", &pgconn.PgError{Code: "40P01"}, false},
		{"other code", &pgconn.PgError{Code: "23505"}, false},
		{"not a database error", errors.New("boom"), false},
		{"nil", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := IsSerializationFailure(tt.err); got != tt.want {
				t.Errorf("IsSerializationFailure() = %v, want %v", got, tt.want)
			}
		})
	}
}
