package payments

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) CreateDeposit(
	ctx context.Context,
	userID uuid.UUID,
	amountMinor int64,
	currency string,
	provider string,
	idempotencyKey string,
) (*Deposit, error) {
	const existingQuery = `
		SELECT id, amount_minor, currency, provider, status,
		       provider_reference, payment_id, completed_at, created_at, updated_at
		FROM wallet_deposits
		WHERE user_id = $1
		  AND idempotency_key = $2
	`

	var existing Deposit
	err := r.db.QueryRow(ctx, existingQuery, userID, idempotencyKey).Scan(
		&existing.ID,
		&existing.AmountMinor,
		&existing.Currency,
		&existing.Provider,
		&existing.Status,
		&existing.ProviderReference,
		&existing.PaymentID,
		&existing.CompletedAt,
		&existing.CreatedAt,
		&existing.UpdatedAt,
	)
	if err == nil {
		if existing.AmountMinor != amountMinor || existing.Currency != currency || existing.Provider != provider {
			return nil, ErrIdempotencyConflict
		}
		return &existing, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("find deposit by idempotency key: %w", err)
	}

	const insertQuery = `
		INSERT INTO wallet_deposits (
			user_id,
			amount_minor,
			currency,
			provider,
			status,
			idempotency_key
		)
		VALUES ($1,$2,$3,$4,'pending',$5)
		RETURNING id, amount_minor, currency, provider, status,
		          provider_reference, payment_id, completed_at, created_at, updated_at
	`

	var deposit Deposit
	err = r.db.QueryRow(
		ctx,
		insertQuery,
		userID,
		amountMinor,
		currency,
		provider,
		idempotencyKey,
	).Scan(
		&deposit.ID,
		&deposit.AmountMinor,
		&deposit.Currency,
		&deposit.Provider,
		&deposit.Status,
		&deposit.ProviderReference,
		&deposit.PaymentID,
		&deposit.CompletedAt,
		&deposit.CreatedAt,
		&deposit.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create deposit: %w", err)
	}

	return &deposit, nil
}

func (r *Repository) ListDeposits(
	ctx context.Context,
	userID uuid.UUID,
	limit int,
) ([]Deposit, error) {
	const query = `
		SELECT id, amount_minor, currency, provider, status,
		       provider_reference, payment_id, completed_at, created_at, updated_at
		FROM wallet_deposits
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`

	rows, err := r.db.Query(ctx, query, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("list deposits: %w", err)
	}
	defer rows.Close()

	result := make([]Deposit, 0)
	for rows.Next() {
		var item Deposit
		if err := rows.Scan(
			&item.ID,
			&item.AmountMinor,
			&item.Currency,
			&item.Provider,
			&item.Status,
			&item.ProviderReference,
			&item.PaymentID,
			&item.CompletedAt,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan deposit: %w", err)
		}
		result = append(result, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate deposits: %w", err)
	}

	return result, nil
}

func (r *Repository) GetDeposit(
	ctx context.Context,
	userID uuid.UUID,
	depositID uuid.UUID,
) (*Deposit, error) {
	const query = `
		SELECT id, amount_minor, currency, provider, status,
		       provider_reference, payment_id, completed_at, created_at, updated_at
		FROM wallet_deposits
		WHERE id = $1
		  AND user_id = $2
	`

	var deposit Deposit
	err := r.db.QueryRow(ctx, query, depositID, userID).Scan(
		&deposit.ID,
		&deposit.AmountMinor,
		&deposit.Currency,
		&deposit.Provider,
		&deposit.Status,
		&deposit.ProviderReference,
		&deposit.PaymentID,
		&deposit.CompletedAt,
		&deposit.CreatedAt,
		&deposit.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrDepositNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get deposit: %w", err)
	}

	return &deposit, nil
}

func (r *Repository) SettleDevelopmentDeposit(
	ctx context.Context,
	depositID uuid.UUID,
	providerReference string,
) (*DepositSettlementResult, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return nil, fmt.Errorf("begin deposit settlement: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var userID uuid.UUID
	var deposit Deposit
	err = tx.QueryRow(
		ctx,
		`SELECT user_id, id, amount_minor, currency, provider, status,
		        provider_reference, payment_id, completed_at, created_at, updated_at
		 FROM wallet_deposits
		 WHERE id = $1
		 FOR UPDATE`,
		depositID,
	).Scan(
		&userID,
		&deposit.ID,
		&deposit.AmountMinor,
		&deposit.Currency,
		&deposit.Provider,
		&deposit.Status,
		&deposit.ProviderReference,
		&deposit.PaymentID,
		&deposit.CompletedAt,
		&deposit.CreatedAt,
		&deposit.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrDepositNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("lock deposit: %w", err)
	}
	if deposit.Provider != ProviderDevelopment && deposit.Provider != "card_transfer" {
		return nil, ErrDepositNotPending
	}

	if deposit.Status == StatusSucceeded {
		if deposit.ProviderReference == nil || *deposit.ProviderReference != providerReference {
			return nil, ErrIdempotencyConflict
		}

		wallet, err := loadWalletInTx(ctx, tx, userID, deposit.Currency)
		if err != nil {
			return nil, err
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("commit repeated deposit settlement: %w", err)
		}
		return &DepositSettlementResult{Deposit: deposit, Wallet: *wallet}, nil
	}
	if deposit.Status != "pending" {
		return nil, ErrDepositNotPending
	}

	var referenceUsed bool
	err = tx.QueryRow(
		ctx,
		`SELECT EXISTS (
			SELECT 1
			FROM wallet_deposits
			WHERE provider = $1
			  AND provider_reference = $2
			  AND id <> $3
		)`,
		deposit.Provider,
		providerReference,
		depositID,
	).Scan(&referenceUsed)
	if err != nil {
		return nil, fmt.Errorf("check provider reference: %w", err)
	}
	if referenceUsed {
		return nil, ErrIdempotencyConflict
	}

	_, err = tx.Exec(
		ctx,
		`INSERT INTO wallet_accounts (user_id, currency)
		 VALUES ($1, $2)
		 ON CONFLICT (user_id, currency) DO NOTHING`,
		userID,
		deposit.Currency,
	)
	if err != nil {
		return nil, fmt.Errorf("ensure wallet account for deposit: %w", err)
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
		deposit.Currency,
	).Scan(&balanceMinor)
	if err != nil {
		return nil, fmt.Errorf("lock wallet for deposit: %w", err)
	}

	payment := Payment{}
	err = tx.QueryRow(
		ctx,
		`INSERT INTO payments (
			user_id,
			method,
			amount_minor,
			currency,
			status,
			idempotency_key,
			provider_reference,
			metadata,
			completed_at
		 ) VALUES ($1, $2, $3, $4, $5, $6, $7,
		           jsonb_build_object('purpose', 'wallet_deposit', 'depositId', $8::text),
		           now())
		 RETURNING id, method, amount_minor, currency, status, created_at, completed_at`,
		userID,
		map[bool]string{true:"card_to_card",false:"gateway"}[deposit.Provider=="card_transfer"],
		deposit.AmountMinor,
		deposit.Currency,
		StatusSucceeded,
		"deposit:"+deposit.ID.String(),
		providerReference,
		deposit.ID,
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
		return nil, fmt.Errorf("create deposit payment: %w", err)
	}

	err = tx.QueryRow(
		ctx,
		`UPDATE wallet_accounts
		 SET balance_minor = balance_minor + $3,
		     updated_at = now()
		 WHERE user_id = $1
		   AND currency = $2
		 RETURNING balance_minor`,
		userID,
		deposit.Currency,
		deposit.AmountMinor,
	).Scan(&balanceMinor)
	if err != nil {
		return nil, fmt.Errorf("credit wallet from deposit: %w", err)
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
		 ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		userID,
		deposit.Currency,
		payment.ID,
		DirectionCredit,
		ReasonWalletDeposit,
		deposit.AmountMinor,
		balanceMinor,
		"deposit:"+deposit.ID.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("record deposit wallet credit: %w", err)
	}

	err = tx.QueryRow(
		ctx,
		`UPDATE wallet_deposits
		 SET status = 'succeeded',
		     provider_reference = $2,
		     payment_id = $3,
		     completed_at = now(),
		     updated_at = now()
		 WHERE id = $1
		 RETURNING status, provider_reference, payment_id, completed_at, updated_at`,
		deposit.ID,
		providerReference,
		payment.ID,
	).Scan(
		&deposit.Status,
		&deposit.ProviderReference,
		&deposit.PaymentID,
		&deposit.CompletedAt,
		&deposit.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("complete deposit: %w", err)
	}

	if deposit.Provider == "card_transfer" {
        tag,e:=tx.Exec(ctx,`UPDATE wallet_card_receipts SET status='approved',reviewed_at=now() WHERE deposit_id=$1 AND status='pending_review'`,deposit.ID)
        if e!=nil{return nil,fmt.Errorf("approve receipt: %w",e)}
        if tag.RowsAffected()!=1{return nil,ErrDepositNotPending}
    }
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit deposit settlement: %w", err)
	}

	return &DepositSettlementResult{
		Deposit: deposit,
		Wallet: Wallet{
			Currency:     deposit.Currency,
			BalanceMinor: balanceMinor,
		},
	}, nil
}

func loadWalletInTx(
	ctx context.Context,
	tx pgx.Tx,
	userID uuid.UUID,
	currency string,
) (*Wallet, error) {
	var wallet Wallet
	err := tx.QueryRow(
		ctx,
		`SELECT currency, balance_minor, updated_at
		 FROM wallet_accounts
		 WHERE user_id = $1
		   AND currency = $2`,
		userID,
		currency,
	).Scan(&wallet.Currency, &wallet.BalanceMinor, &wallet.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("load wallet after deposit settlement: %w", err)
	}
	return &wallet, nil
}
