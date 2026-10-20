package payments

import (
 "encoding/json"
 "errors"
 "io"
 "math/big"
 "net/http"
 "strings"
 "time"

 "backend/internal/requestcontext"
 "github.com/go-chi/chi/v5"
 "github.com/google/uuid"
 "github.com/jackc/pgx/v5"
)

type discountQuote struct {
 Code string `json:"code"`
 DiscountMinor int64 `json:"discountMinor"`
 PayableMinor int64 `json:"payableMinor"`
 Currency string `json:"currency"`
}

func (h *Handler) quoteDiscount(r *http.Request, userID uuid.UUID, subscriptionID uuid.UUID, code string) (discountQuote, error) {
 var amount int64
 var currency string
 err := h.service.repository.db.QueryRow(r.Context(),
  "SELECT price_minor, currency FROM user_subscriptions WHERE id=$1 AND user_id=$2 AND status='pending_payment'",
  subscriptionID,userID).Scan(&amount,&currency)
 if err != nil { return discountQuote{},err }
 q := discountQuote{PayableMinor:amount,Currency:currency}
 code = strings.ToUpper(strings.TrimSpace(code))
 if code == "" {return q,nil}
 var typ, discountCurrency string
 var value int64
 var starts, expires *time.Time
 var maxUses *int
 var uses int
 err = h.service.repository.db.QueryRow(r.Context(),
  "SELECT discount_type, discount_value, COALESCE(currency,''), starts_at, expires_at, max_redemptions, redemptions FROM discount_codes WHERE code=$1 AND active=true",
  code).Scan(&typ,&value,&discountCurrency,&starts,&expires,&maxUses,&uses)
 if err != nil {return discountQuote{},err}
 now:=time.Now()
 if starts!=nil && now.Before(*starts) || expires!=nil && !now.Before(*expires) || maxUses!=nil && uses>=*maxUses {return discountQuote{},errors.New("discount unavailable")}
 if typ=="percent" {q.DiscountMinor=percentageDiscount(amount,value)} else if discountCurrency==currency {q.DiscountMinor=value} else {return discountQuote{},errors.New("discount currency mismatch")}
 if q.DiscountMinor>amount {q.DiscountMinor=amount}
 q.PayableMinor=amount-q.DiscountMinor
 q.Code=code
 return q,nil
}

func (h *Handler) DiscountQuote(w http.ResponseWriter,r *http.Request) {
 userID,ok:=requestcontext.UserID(r.Context());if !ok {writeJSONError(w,401,"authentication required");return}
 id,err:=uuid.Parse(chi.URLParam(r,"subscriptionID"));if err!=nil {writeJSONError(w,400,"invalid subscription");return}
 var body struct{Code string `json:"code"`}
 if err=json.NewDecoder(http.MaxBytesReader(w,r.Body,4096)).Decode(&body);err!=nil {writeJSONError(w,400,"invalid request");return}
 q,err:=h.quoteDiscount(r,userID,id,body.Code)
 if err!=nil {writeJSONError(w,400,"invalid or unavailable discount");return}
 writeJSON(w,200,q)
}

func (h *Handler) SubmitCardReceipt(w http.ResponseWriter,r *http.Request) {
 userID,ok:=requestcontext.UserID(r.Context());if !ok {writeJSONError(w,401,"authentication required");return}
 id,err:=uuid.Parse(chi.URLParam(r,"subscriptionID"));if err!=nil {writeJSONError(w,400,"invalid subscription");return}
 r.Body=http.MaxBytesReader(w,r.Body,6<<20)
 if err=r.ParseMultipartForm(6<<20);err!=nil {writeJSONError(w,400,"invalid receipt upload");return}
 file,header,err:=r.FormFile("receipt");if err!=nil {writeJSONError(w,400,"receipt required");return};defer file.Close()
 b,err:=io.ReadAll(io.LimitReader(file,(5<<20)+1));if err!=nil || len(b)==0 || len(b)>5<<20 {writeJSONError(w,400,"receipt must be at most 5 MB");return}
 contentType:=http.DetectContentType(b)
 if contentType!="image/jpeg" && contentType!="image/png" && contentType!="image/webp" && contentType!="application/pdf" {writeJSONError(w,400,"unsupported receipt type");return}
 // Lock the subscription to serialize submissions and prevent post-payment receipts.
 tx,err:=h.service.repository.db.Begin(r.Context());if err!=nil {writeJSONError(w,500,"database error");return};defer tx.Rollback(r.Context())
 var price int64;var currency string
 err=tx.QueryRow(r.Context(),"SELECT price_minor,currency FROM user_subscriptions WHERE id=$1 AND user_id=$2 AND status='pending_payment' FOR UPDATE",id,userID).Scan(&price,&currency)
 if err!=nil {writeJSONError(w,409,"subscription is not pending");return}
 cardID,err:=uuid.Parse(strings.TrimSpace(r.FormValue("paymentCardId")))
 if err!=nil {writeJSONError(w,400,"select a valid payment card");return}
 var cardActive bool
 if err=tx.QueryRow(r.Context(),"SELECT active FROM payment_cards WHERE id=$1",cardID).Scan(&cardActive);err!=nil || !cardActive {writeJSONError(w,400,"payment card is unavailable");return}
 code:=strings.ToUpper(strings.TrimSpace(r.FormValue("discountCode")))
 var discountID *uuid.UUID;var reduction int64
 if code!="" {
  var did uuid.UUID;var typ,dc string;var value int64;var starts,expires *time.Time;var maxUses *int;var uses int
  err=tx.QueryRow(r.Context(),"SELECT id,discount_type,discount_value,COALESCE(currency,''),starts_at,expires_at,max_redemptions,redemptions FROM discount_codes WHERE code=$1 AND active=true FOR UPDATE",code).Scan(&did,&typ,&value,&dc,&starts,&expires,&maxUses,&uses)
  now:=time.Now()
  if err!=nil || starts!=nil && now.Before(*starts) || expires!=nil && !now.Before(*expires) || maxUses!=nil && uses>=*maxUses {writeJSONError(w,400,"discount unavailable");return}
  if typ=="percent" {reduction=percentageDiscount(price,value)} else if dc==currency {reduction=value} else {writeJSONError(w,400,"discount currency mismatch");return}
  if reduction>price {reduction=price};discountID=&did
 }
 var receiptID uuid.UUID
 err=tx.QueryRow(r.Context(),`INSERT INTO card_transfer_receipts(user_id,subscription_id,filename,content_type,receipt_bytes,discount_id,discount_minor,payable_minor,payment_card_id)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id`,userID,id,header.Filename,contentType,b,discountID,reduction,price-reduction,cardID).Scan(&receiptID)
 if err!=nil {writeJSONError(w,409,"receipt already submitted or could not be saved");return}
 if discountID!=nil {
  _,err=tx.Exec(r.Context(),"UPDATE discount_codes SET redemptions=redemptions+1 WHERE id=$1",*discountID);if err!=nil {writeJSONError(w,500,"database error");return}
  _,err=tx.Exec(r.Context(),"INSERT INTO discount_redemptions(discount_id,user_id,subscription_id,amount_minor) VALUES($1,$2,$3,$4)",*discountID,userID,id,reduction);if err!=nil {writeJSONError(w,500,"database error");return}
 }
 if err=tx.Commit(r.Context());err!=nil {writeJSONError(w,500,"database error");return}
 writeJSON(w,http.StatusCreated,map[string]any{"id":receiptID,"status":"pending_review","discountMinor":reduction,"payableMinor":price-reduction,"currency":currency})
}

// This endpoint returns receipt metadata only; never expose stored receipt bytes.
func (h *Handler) CardReceipt(w http.ResponseWriter,r *http.Request) {
 userID,ok:=requestcontext.UserID(r.Context());if !ok {writeJSONError(w,401,"authentication required");return}
 id,err:=uuid.Parse(chi.URLParam(r,"subscriptionID"));if err!=nil {writeJSONError(w,400,"invalid subscription");return}
 var receiptID uuid.UUID;var status,currency string;var discount,payable int64
 err=h.service.repository.db.QueryRow(r.Context(),`SELECT c.id,c.status,s.currency,c.discount_minor,c.payable_minor
 FROM card_transfer_receipts c JOIN user_subscriptions s ON s.id=c.subscription_id
 WHERE c.subscription_id=$1 AND c.user_id=$2`,id,userID).Scan(&receiptID,&status,&currency,&discount,&payable)
 if errors.Is(err,pgx.ErrNoRows) {writeJSONError(w,404,"receipt not found");return}
 if err!=nil {writeJSONError(w,500,"database error");return}
 writeJSON(w,200,map[string]any{"id":receiptID,"status":status,"currency":currency,"discountMinor":discount,"payableMinor":payable})
}

// percentageDiscount avoids int64 overflow when computing basis-point discounts.
func percentageDiscount(amount, basisPoints int64) int64 {
 if amount <= 0 || basisPoints <= 0 { return 0 }
 n := new(big.Int).Mul(big.NewInt(amount), big.NewInt(basisPoints))
 n.Div(n, big.NewInt(10000))
 if !n.IsInt64() || n.Int64() > amount { return amount }
 return n.Int64()
}
