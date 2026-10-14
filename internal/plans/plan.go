package plans

import (
 "context"
 "fmt"
 "time"

 "github.com/google/uuid"
 "github.com/jackc/pgx/v5"
 "github.com/jackc/pgx/v5/pgxpool"
)

type Plan struct {
 ID uuid.UUID `json:"id"`
 Code string `json:"code"`
 Name string `json:"name"`
 Description string `json:"description"`
 PriceMinor int64 `json:"priceMinor"`
 Currency string `json:"currency"`
 DurationDays int `json:"durationDays"`
 DataLimitBytes int64 `json:"dataLimitBytes"`
 CreatedAt time.Time `json:"createdAt"`
}

type Repository struct { db *pgxpool.Pool }
func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db:db} }

func (r *Repository) ListActive(ctx context.Context) ([]Plan,error) {
 const query = `SELECT id,code,name,description,price_minor,currency,duration_days,data_limit_bytes,created_at FROM plans WHERE active = TRUE ORDER BY sort_order, price_minor, code`
 rows,err:=r.db.Query(ctx,query)
 if err!=nil {return nil,fmt.Errorf("list plans: %w",err)}
 defer rows.Close()
 result:=make([]Plan,0)
 for rows.Next() {
  var p Plan
  if err:=rows.Scan(&p.ID,&p.Code,&p.Name,&p.Description,&p.PriceMinor,&p.Currency,&p.DurationDays,&p.DataLimitBytes,&p.CreatedAt);err!=nil {return nil,fmt.Errorf("scan plan: %w",err)}
  result=append(result,p)
 }
 if err:=rows.Err();err!=nil{return nil,fmt.Errorf("iterate plans: %w",err)}
 return result,nil
}

func (r *Repository) GetActiveByCode(ctx context.Context,code string)(*Plan,error){
 const query = `SELECT id,code,name,description,price_minor,currency,duration_days,data_limit_bytes,created_at FROM plans WHERE code=$1 AND active=TRUE`
 var p Plan
 err:=r.db.QueryRow(ctx,query,code).Scan(&p.ID,&p.Code,&p.Name,&p.Description,&p.PriceMinor,&p.Currency,&p.DurationDays,&p.DataLimitBytes,&p.CreatedAt)
 if err!=nil {if err==pgx.ErrNoRows{return nil,nil};return nil,fmt.Errorf("get plan: %w",err)}
 return &p,nil
}
