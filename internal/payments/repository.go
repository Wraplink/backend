package payments

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	if db == nil {
		panic("payments: nil database")
	}

	return &Repository{db: db}
}

func (r *Repository) GetWallet(
	ctx context.Context,
	userID uuid.UUID,
	currency string,
) (*Wallet, error) {
	const query = `
		SELECT currency, balance_minor, updated_at
		FROM wallet_accounts
		WHERE user_id = $1
		  AND currency = $2
	`

	var wallet Wallet
	var updatedAt = wallet.UpdatedAt

	err := r.db.QueryRow(ctx, query, userID, currency).Scan(
		&wallet.Currency,
		&wallet.BalanceMinor,
		&updatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return &Wallet{
			Currency:     currency,
			BalanceMinor: 0,
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get wallet: %w", err)
	}

	wallet.UpdatedAt = updatedAt
	return &wallet, nil
}

func (r *Repository) ListTransactions(
	ctx context.Context,
	userID uuid.UUID,
	currency string,
	limit int,
) ([]WalletTransaction, error) {
	const query = `
		SELECT
			id,
			currency,
			payment_id,
			direction,
			reason,
			amount_minor,
			balance_after_minor,
			created_at
		FROM wallet_transactions
		WHERE user_id = $1
		  AND currency = $2
		ORDER BY created_at DESC
		LIMIT $3
	`

	rows, err := r.db.Query(ctx, query, userID, currency, limit)
	if err != nil {
		return nil, fmt.Errorf("list wallet transactions: %w", err)
	}
	defer rows.Close()

	result := make([]WalletTransaction, 0)
	for rows.Next() {
		var item WalletTransaction
		if err := rows.Scan(
			&item.ID,
			&item.Currency,
			&item.PaymentID,
			&item.Direction,
			&item.Reason,
			&item.AmountMinor,
			&item.BalanceAfterMinor,
			&item.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan wallet transaction: %w", err)
		}

		result = append(result, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate wallet transactions: %w", err)
	}

	return result, nil
}

func (r *Repository) ListPayments(
	ctx context.Context,
	userID uuid.UUID,
	limit int,
) ([]Payment, error) {
	const query = `
		SELECT
			id,
			subscription_id,
			method,
			amount_minor,
			currency,
			status,
			created_at,
			completed_at
		FROM payments
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`

	rows, err := r.db.Query(ctx, query, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("list payments: %w", err)
	}
	defer rows.Close()

	result := make([]Payment, 0)
	for rows.Next() {
		var item Payment
		if err := rows.Scan(
			&item.ID,
			&item.SubscriptionID,
			&item.Method,
			&item.AmountMinor,
			&item.Currency,
			&item.Status,
			&item.CreatedAt,
			&item.CompletedAt,
		); err != nil {
			return nil, fmt.Errorf("scan payment: %w", err)
		}

		result = append(result, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate payments: %w", err)
	}

	return result, nil
}

func (r *Repository) GetPayment(
	ctx context.Context,
	userID uuid.UUID,
	paymentID uuid.UUID,
) (*Payment, error) {
	const query = `
		SELECT
			id,
			subscription_id,
			method,
			amount_minor,
			currency,
			status,
			created_at,
			completed_at
		FROM payments
		WHERE id = $1
		  AND user_id = $2
	`

	var payment Payment
	err := r.db.QueryRow(ctx, query, paymentID, userID).Scan(
		&payment.ID,
		&payment.SubscriptionID,
		&payment.Method,
		&payment.AmountMinor,
		&payment.Currency,
		&payment.Status,
		&payment.CreatedAt,
		&payment.CompletedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrPaymentNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get payment: %w", err)
	}

	return &payment, nil
}

func (r *Repository) PaySubscriptionWithWallet(
	ctx context.Context,
	userID uuid.UUID,
	subscriptionID uuid.UUID,
	idempotencyKey string,
) (*PaymentResult, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return nil, fmt.Errorf("begin wallet payment: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	existing, err := findPaymentByIdempotencyKey(ctx, tx, userID, idempotencyKey)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		if existing.SubscriptionID == nil || *existing.SubscriptionID != subscriptionID {
			return nil, ErrIdempotencyConflict
		}
		if existing.Status != StatusSucceeded {
			return nil, ErrIdempotencyConflict
		}

		result, err := loadPaymentResult(ctx, tx, *existing, subscriptionID)
		if err != nil {
			return nil, err
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("commit idempotent wallet payment: %w", err)
		}
		return result, nil
	}

	_, err = tx.Exec(
		ctx,
		`UPDATE user_subscriptions
		 SET status = 'expired', updated_at = now()
		 WHERE user_id = $1
		   AND status = 'active'
		   AND expires_at <= now()`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("expire ended subscriptions: %w", err)
	}

	var amountMinor int64
	var currency string
	var subscriptionStatus string

	err = tx.QueryRow(
		ctx,
		`SELECT price_minor, currency, status
		 FROM user_subscriptions
		 WHERE id = $1
		   AND user_id = $2
		 FOR UPDATE`,
		subscriptionID,
		userID,
	).Scan(&amountMinor, &currency, &subscriptionStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSubscriptionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("lock subscription for payment: %w", err)
	}
	if subscriptionStatus != "pending_payment" {
		return nil, ErrSubscriptionNotPending
	}

	var receiptSubmitted bool
	if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM card_transfer_receipts WHERE subscription_id=$1)", subscriptionID).Scan(&receiptSubmitted); err != nil {
		return nil, fmt.Errorf("check receipt submission: %w", err)
	}
	if receiptSubmitted {
		return nil, ErrSubscriptionNotPending
	}


	_, err = tx.Exec(
		ctx,
		`INSERT INTO wallet_accounts (user_id, currency)
		 VALUES ($1, $2)
		 ON CONFLICT (user_id, currency) DO NOTHING`,
		userID,
		currency,
	)
	if err != nil {
		return nil, fmt.Errorf("ensure wallet account: %w", err)
	}

	var balanceMinor int64
	err = tx.QueryRow(
		ctx,
		`SELECT balance_minor
		 FROM wallet_accounts
		 WHERE user_id = $1
		   AND currency = $2
		 FOR UPDATE`,
		userID,
		currency,
	).Scan(&balanceMinor)
	if err != nil {
		return nil, fmt.Errorf("lock wallet account: %w", err)
	}
	if balanceMinor < amountMinor {
		return nil, ErrInsufficientWalletBalance
	}

	payment := Payment{
		SubscriptionID: &subscriptionID,
	}
	err = tx.QueryRow(
		ctx,
		`INSERT INTO payments (
			user_id,
			subscription_id,
			method,
			amount_minor,
			currency,
			status,
			idempotency_key,
			completed_at
		 ) VALUES ($1, $2, $3, $4, $5, $6, $7, now())
		 RETURNING id, method, amount_minor, currency, status, created_at, completed_at`,
		userID,
		subscriptionID,
		MethodWallet,
		amountMinor,
		currency,
		StatusSucceeded,
		idempotencyKey,
	).Scan(
		&payment.ID,
		&payment.Method,
		&payment.AmountMinor,
		&payment.Currency,
		&payment.Status,
		&payment.CreatedAt,
		&payment.CompletedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create wallet payment: %w", err)
	}

	if amountMinor > 0 {
		err = tx.QueryRow(
			ctx,
			`UPDATE wallet_accounts
			 SET balance_minor = balance_minor - $3,
			     updated_at = now()
			 WHERE user_id = $1
			   AND currency = $2
			 RETURNING balance_minor`,
			userID,
			currency,
			amountMinor,
		).Scan(&balanceMinor)
		if err != nil {
			return nil, fmt.Errorf("debit wallet: %w", err)
		}

		_, err = tx.Exec(
			ctx,
			`INSERT INTO wallet_transactions (
				user_id,
				currency,
				payment_id,
				direction,
				reason,
				amount_minor,
				balance_after_minor,
				idempotency_key
			 ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			userID,
			currency,
			payment.ID,
			DirectionDebit,
			ReasonSubscriptionPayment,
			amountMinor,
			balanceMinor,
			"payment:"+payment.ID.String(),
		)
		if err != nil {
			return nil, fmt.Errorf("record wallet debit: %w", err)
		}
	}

	result := &PaymentResult{
		Payment:            payment,
		SubscriptionID:     subscriptionID,
		SubscriptionStatus: "confirmed",
	}

	err = tx.QueryRow(
		ctx,
		`UPDATE user_subscriptions
		 SET status = 'confirmed', updated_at = now()
		 WHERE id = $1
		   AND user_id = $2
		   AND status = 'pending_payment'
		 RETURNING starts_at, expires_at`,
		subscriptionID,
		userID,
	).Scan(&result.StartsAt, &result.ExpiresAt)
	if err != nil {
		return nil, fmt.Errorf("activate paid subscription: %w", err)
	}

	if _, err := tx.Exec(ctx, "SELECT promote_user_subscription($1)", userID); err != nil {
		return nil, fmt.Errorf("promote confirmed subscription: %w", err)
	}
	if err := tx.QueryRow(ctx, "SELECT status, starts_at, expires_at FROM user_subscriptions WHERE id=$1", subscriptionID).Scan(&result.SubscriptionStatus, &result.StartsAt, &result.ExpiresAt); err != nil {
		return nil, fmt.Errorf("load confirmed subscription: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit wallet payment: %w", err)
	}

	return result, nil
}

func findPaymentByIdempotencyKey(
	ctx context.Context,
	tx pgx.Tx,
	userID uuid.UUID,
	idempotencyKey string,
) (*Payment, error) {
	const query = `
		SELECT
			id,
			subscription_id,
			method,
			amount_minor,
			currency,
			status,
			created_at,
			completed_at
		FROM payments
		WHERE user_id = $1
		  AND idempotency_key = $2
	`

	var payment Payment
	err := tx.QueryRow(ctx, query, userID, idempotencyKey).Scan(
		&payment.ID,
		&payment.SubscriptionID,
		&payment.Method,
		&payment.AmountMinor,
		&payment.Currency,
		&payment.Status,
		&payment.CreatedAt,
		&payment.CompletedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find payment by idempotency key: %w", err)
	}

	return &payment, nil
}

func loadPaymentResult(
	ctx context.Context,
	tx pgx.Tx,
	payment Payment,
	subscriptionID uuid.UUID,
) (*PaymentResult, error) {
	result := &PaymentResult{
		Payment:        payment,
		SubscriptionID: subscriptionID,
	}

	err := tx.QueryRow(
		ctx,
		`SELECT status, starts_at, expires_at
		 FROM user_subscriptions
		 WHERE id = $1`,
		subscriptionID,
	).Scan(
		&result.SubscriptionStatus,
		&result.StartsAt,
		&result.ExpiresAt,
	)
	if err != nil {
		return nil, fmt.Errorf("load paid subscription: %w", err)
	}

	return result, nil
}
