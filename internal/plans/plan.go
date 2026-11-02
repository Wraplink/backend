package plans

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Plan struct {
	ID             uuid.UUID `json:"id"`
	Code           string    `json:"code"`
	Name           string    `json:"name"`
	Description    string    `json:"description"`
	PriceMinor     int64     `json:"priceMinor"`
	Currency       string    `json:"currency"`
	DurationDays   int       `json:"durationDays"`
	DataLimitBytes int64     `json:"dataLimitBytes"`
	CreatedAt      time.Time `json:"createdAt"`
	Active bool `json:"active"`
	PublishAt *time.Time `json:"publishAt"`
	SortOrder int `json:"sortOrder"`
}

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	if db == nil {
		panic("plans: nil database")
	}

	return &Repository{db: db}
}

func (r *Repository) ListActive(ctx context.Context) ([]Plan, error) {
	const query = `
		SELECT
			id,
			code,
			name,
			description,
			price_minor,
			currency,
			duration_days,
			data_limit_bytes,
			created_at
		FROM plans
		WHERE active = TRUE AND (publish_at IS NULL OR publish_at <= now())
		ORDER BY sort_order, price_minor, code
	`

	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list plans: %w", err)
	}
	defer rows.Close()

	result := make([]Plan, 0)

	for rows.Next() {
		var plan Plan

		if err := rows.Scan(
			&plan.ID,
			&plan.Code,
			&plan.Name,
			&plan.Description,
			&plan.PriceMinor,
			&plan.Currency,
			&plan.DurationDays,
			&plan.DataLimitBytes,
			&plan.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan plan: %w", err)
		}

		result = append(result, plan)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate plans: %w", err)
	}

	return result, nil
}

func (r *Repository) GetActiveByCode(
	ctx context.Context,
	code string,
) (*Plan, error) {
	const query = `
		SELECT
			id,
			code,
			name,
			description,
			price_minor,
			currency,
			duration_days,
			data_limit_bytes,
			created_at
		FROM plans
		WHERE code = $1
		  AND active = TRUE
          AND (publish_at IS NULL OR publish_at <= now())
	`

	var plan Plan

	err := r.db.QueryRow(ctx, query, code).Scan(
		&plan.ID,
		&plan.Code,
		&plan.Name,
		&plan.Description,
		&plan.PriceMinor,
		&plan.Currency,
		&plan.DurationDays,
		&plan.DataLimitBytes,
		&plan.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get plan: %w", err)
	}

	return &plan, nil
}

type AdminPlanInput struct {
 Code string `json:"code"`
 Name string `json:"name"`
 Description string `json:"description"`
 PriceMinor int64 `json:"priceMinor"`
 Currency string `json:"currency"`
 DurationDays int `json:"durationDays"`
 DataLimitBytes int64 `json:"dataLimitBytes"`
 Active bool `json:"active"`
 PublishAt *time.Time `json:"publishAt"`
 SortOrder int `json:"sortOrder"`
}

func (r *Repository) ListAdmin(ctx context.Context) ([]Plan,error) {
 rows,err:=r.db.Query(ctx,`SELECT id,code,name,description,price_minor,currency,duration_days,data_limit_bytes,created_at,active,publish_at,sort_order FROM plans ORDER BY sort_order,code`)
 if err!=nil{return nil,err};defer rows.Close()
 items:=make([]Plan,0)
 for rows.Next(){var p Plan;if err:=rows.Scan(&p.ID,&p.Code,&p.Name,&p.Description,&p.PriceMinor,&p.Currency,&p.DurationDays,&p.DataLimitBytes,&p.CreatedAt,&p.Active,&p.PublishAt,&p.SortOrder);err!=nil{return nil,err};items=append(items,p)}
 return items,rows.Err()
}
func (r *Repository) SaveAdmin(ctx context.Context, p AdminPlanInput) error {
 _,err:=r.db.Exec(ctx,`INSERT INTO plans(code,name,description,price_minor,currency,duration_days,data_limit_bytes,active,publish_at,sort_order)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
 ON CONFLICT(code) DO UPDATE SET name=EXCLUDED.name,description=EXCLUDED.description,price_minor=EXCLUDED.price_minor,currency=EXCLUDED.currency,duration_days=EXCLUDED.duration_days,data_limit_bytes=EXCLUDED.data_limit_bytes,active=EXCLUDED.active,publish_at=EXCLUDED.publish_at,sort_order=EXCLUDED.sort_order`,
 p.Code,p.Name,p.Description,p.PriceMinor,p.Currency,p.DurationDays,p.DataLimitBytes,p.Active,p.PublishAt,p.SortOrder)
 return err
}
