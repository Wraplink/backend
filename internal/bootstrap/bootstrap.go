package bootstrap

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

const gib int64 = 1024 * 1024 * 1024

type planSeed struct {
	Code           string
	Name           string
	Description    string
	PriceMinor     int64
	Currency       string
	DurationDays   int
	DataLimitBytes int64
	SortOrder      int
}

var defaultPlans = []planSeed{
	{
		Code:           "starter",
		Name:           "Starter",
		Description:    "Starter plan with 50 GB traffic for 30 days.",
		PriceMinor:     1_000_000,
		Currency:       "IRR",
		DurationDays:   30,
		DataLimitBytes: 50 * gib,
		SortOrder:      10,
	},
	{
		Code:           "gamer",
		Name:           "Gamer",
		Description:    "Gamer plan with 150 GB traffic for 30 days.",
		PriceMinor:     2_500_000,
		Currency:       "IRR",
		DurationDays:   30,
		DataLimitBytes: 150 * gib,
		SortOrder:      20,
	},
	{
		Code:           "pro",
		Name:           "Pro",
		Description:    "Pro plan with 500 GB traffic for 30 days.",
		PriceMinor:     5_000_000,
		Currency:       "IRR",
		DurationDays:   30,
		DataLimitBytes: 500 * gib,
		SortOrder:      30,
	},
}

// EnsureRequiredData guarantees that reference data required by the application
// exists before the HTTP server starts.
//
// Seeds are intentionally insert-only. Existing rows are never overwritten, so
// operational changes made later through SQL/admin tooling survive restarts.
func EnsureRequiredData(
	ctx context.Context,
	db *pgxpool.Pool,
	logger *slog.Logger,
) error {
	if db == nil {
		return fmt.Errorf("bootstrap: nil database")
	}
	if logger == nil {
		logger = slog.Default()
	}

	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("bootstrap: begin transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	const insertPlan = `
		INSERT INTO plans (
			code,
			name,
			description,
			price_minor,
			currency,
			duration_days,
			data_limit_bytes,
			active,
			sort_order
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,TRUE,$8)
		ON CONFLICT (code) DO NOTHING
	`

	inserted := 0

	for _, plan := range defaultPlans {
		result, err := tx.Exec(
			ctx,
			insertPlan,
			plan.Code,
			plan.Name,
			plan.Description,
			plan.PriceMinor,
			plan.Currency,
			plan.DurationDays,
			plan.DataLimitBytes,
			plan.SortOrder,
		)
		if err != nil {
			return fmt.Errorf("bootstrap: ensure plan %q: %w", plan.Code, err)
		}

		inserted += int(result.RowsAffected())
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("bootstrap: commit transaction: %w", err)
	}

	logger.Info(
		"reference data bootstrap completed",
		"plansConfigured", len(defaultPlans),
		"plansInserted", inserted,
	)

	return nil
}
