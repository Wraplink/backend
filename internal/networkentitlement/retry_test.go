package networkentitlement

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestIsRetryableTransactionError(t *testing.T) {
	tests := []struct {
		name string
		err error
		want bool
	}{
		{name: "serialization failure", err: &pgconn.PgError{Code: "40001"}, want: true},
		{name: "deadlock detected", err: fmt.Errorf("commit failed: %w", &pgconn.PgError{Code: "40P01"}), want: true},
		{name: "unique constraint violation", err: &pgconn.PgError{Code: "23505"}},
		{name: "unrelated error", err: errors.New("connection failed")},
		{name: "nil error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isRetryableTransactionError(tt.err); got != tt.want {
				t.Fatalf("isRetryableTransactionError(%v) = %t, want %t", tt.err, got, tt.want)
			}
		})
	}
}
