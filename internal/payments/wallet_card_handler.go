package payments

import (
 "io"
 "fmt"
 "encoding/json"
 "net/http"
 "strings"
 "errors"
 "github.com/go-chi/chi/v5"
 "github.com/google/uuid"
 "github.com/jackc/pgx/v5"
)
func(h *DepositHandler) SubmitCardReceipt(w http.ResponseWriter,r *http.Request){
 userID,ok:=authenticatedUserID(w,r);if !ok{return}
 r.Body=http.MaxBytesReader(w,r.Body,6<<20)
 if r.ParseMultipartForm(6<<20)!=nil{writeJSONError(w,400,"invalid upload");return}
 file,header,e:=r.FormFile("receipt");if e!=nil{writeJSONError(w,400,"receipt required");return};defer file.Close()
 bytes,e:=io.ReadAll(io.LimitReader(file,5<<20+1));if e!=nil||len(bytes)==0||len(bytes)>5<<20{writeJSONError(w,400,"receipt must be 1–5 MB");return}
 typ:=http.DetectContentType(bytes)
 if typ!="image/jpeg"&&typ!="image/png"&&typ!="image/webp"&&typ!="application/pdf"{writeJSONError(w,400,"unsupported receipt format");return}
 cardID,e:=uuid.Parse(strings.TrimSpace(r.FormValue("paymentCardId")));if e!=nil{writeJSONError(w,400,"invalid destination card");return}
 amountText:=strings.TrimSpace(r.FormValue("amountMinor"))
 var amount int64
 if _,e=fmt.Sscan(amountText,&amount);e!=nil||amount<=0||strings.ContainsAny(amountText," +-eE."){writeJSONError(w,400,"invalid amount");return}
 key:=strings.TrimSpace(r.Header.Get("Idempotency-Key"));if key==""||len(key)>100{writeJSONError(w,400,"Idempotency-Key required");return}
 tx,e:=h.service.repository.db.Begin(r.Context());if e!=nil{writeJSONError(w,500,"database error");return};defer tx.Rollback(r.Context())
 var active bool
 if tx.QueryRow(r.Context(),"SELECT active FROM payment_cards WHERE id=$1",cardID).Scan(&active)!=nil||!active{writeJSONError(w,400,"destination card unavailable");return}
 var id uuid.UUID;var existingAmount int64;var existingProvider string
 e=tx.QueryRow(r.Context(),"SELECT id,amount_minor,provider FROM wallet_deposits WHERE user_id=$1 AND idempotency_key=$2 FOR UPDATE",userID,key).Scan(&id,&existingAmount,&existingProvider)
 if e==nil{
  if existingAmount!=amount||existingProvider!="card_transfer"{writeJSONError(w,409,"idempotency conflict");return}
  var existingCard uuid.UUID
  if tx.QueryRow(r.Context(),"SELECT payment_card_id FROM wallet_card_receipts WHERE deposit_id=$1",id).Scan(&existingCard)!=nil||existingCard!=cardID{writeJSONError(w,409,"receipt already submitted with different details");return}
  writeJSON(w,200,map[string]any{"id":id,"status":"pending_review"});return
 }
 if !errors.Is(e,pgx.ErrNoRows){writeJSONError(w,500,"database error");return}
 e=tx.QueryRow(r.Context(),"INSERT INTO wallet_deposits(user_id,amount_minor,currency,provider,status,idempotency_key) VALUES($1,$2,'IRR','card_transfer','pending',$3) RETURNING id",userID,amount,key).Scan(&id)
 if e!=nil{writeJSONError(w,409,"deposit request conflict");return}
 _,e=tx.Exec(r.Context(),"INSERT INTO wallet_card_receipts(deposit_id,user_id,payment_card_id,filename,content_type,receipt_bytes) VALUES($1,$2,$3,$4,$5,$6)",id,userID,cardID,header.Filename,typ,bytes)
 if e!=nil{writeJSONError(w,500,"could not store receipt");return}
 if tx.Commit(r.Context())!=nil{writeJSONError(w,500,"could not submit receipt");return}
 writeJSON(w,201,map[string]any{"id":id,"status":"pending_review"})
}
func(h *DepositHandler) ReviewAdmin(w http.ResponseWriter,r *http.Request)(uuid.UUID,bool){
 reviewer,ok:=authenticatedUserID(w,r);if !ok{return uuid.Nil,false}
 var allowed bool
 if h.service.repository.db.QueryRow(r.Context(),"SELECT EXISTS(SELECT 1 FROM payment_review_admins WHERE user_id=$1)",reviewer).Scan(&allowed)!=nil{writeJSONError(w,500,"authorization unavailable");return uuid.Nil,false}
 if !allowed{writeJSONError(w,403,"reviewer permission required");return uuid.Nil,false};return reviewer,true
}
func(h *DepositHandler) PendingCardReceipts(w http.ResponseWriter,r *http.Request){
 _,ok:=h.ReviewAdmin(w,r);if !ok{return}
 rows,e:=h.service.repository.db.Query(r.Context(),`SELECT c.id,c.deposit_id,c.filename,c.content_type,d.amount_minor,d.currency,c.created_at,u.email,u.full_name FROM wallet_card_receipts c JOIN wallet_deposits d ON d.id=c.deposit_id JOIN users u ON u.id=c.user_id WHERE c.status='pending_review' ORDER BY c.created_at DESC LIMIT 100`)
 if e!=nil{writeJSONError(w,500,"could not list receipts");return};defer rows.Close()
 items:=make([]map[string]any,0)
 for rows.Next(){var id,deposit uuid.UUID;var filename,typ,currency,email,name string;var amount int64;var created any
  if rows.Scan(&id,&deposit,&filename,&typ,&amount,&currency,&created,&email,&name)!=nil{writeJSONError(w,500,"could not read receipts");return}
  items=append(items,map[string]any{"id":id,"depositId":deposit,"filename":filename,"contentType":typ,"amountMinor":amount,"currency":currency,"createdAt":created,"email":email,"fullName":name})
 }
 if rows.Err()!=nil{writeJSONError(w,500,"could not read receipts");return}
 writeJSON(w,200,map[string]any{"receipts":items})
}
func(h *DepositHandler) CardReceiptAttachment(w http.ResponseWriter,r *http.Request){
 _,ok:=h.ReviewAdmin(w,r);if !ok{return}
 id,e:=uuid.Parse(chi.URLParam(r,"receiptID"));if e!=nil{writeJSONError(w,400,"invalid receipt");return}
 var b []byte;var typ string
 e=h.service.repository.db.QueryRow(r.Context(),"SELECT receipt_bytes,content_type FROM wallet_card_receipts WHERE id=$1",id).Scan(&b,&typ)
 if errors.Is(e,pgx.ErrNoRows){writeJSONError(w,404,"receipt not found");return};if e!=nil{writeJSONError(w,500,"database error");return}
 w.Header().Set("Content-Type",typ);w.Header().Set("Content-Disposition","attachment; filename=wallet-receipt");w.Header().Set("X-Content-Type-Options","nosniff");w.Write(b)
}
func(h *DepositHandler) DecideCardReceipt(w http.ResponseWriter,r *http.Request){
 reviewer,ok:=h.ReviewAdmin(w,r);if !ok{return}
 id,e:=uuid.Parse(chi.URLParam(r,"receiptID"));if e!=nil{writeJSONError(w,400,"invalid receipt");return}
 var body struct{Decision string `json:"decision"`}
 if json.NewDecoder(http.MaxBytesReader(w,r.Body,2048)).Decode(&body)!=nil||(body.Decision!="approved"&&body.Decision!="rejected"){writeJSONError(w,400,"invalid decision");return}
 var depositID uuid.UUID
 e=h.service.repository.db.QueryRow(r.Context(),"SELECT deposit_id FROM wallet_card_receipts WHERE id=$1 AND status='pending_review'",id).Scan(&depositID)
 if errors.Is(e,pgx.ErrNoRows){writeJSONError(w,409,"receipt already reviewed");return};if e!=nil{writeJSONError(w,500,"database error");return}
 if body.Decision=="approved"{
  result,e:=h.service.SettleDevelopmentDeposit(r.Context(),depositID,"wallet-card:"+id.String())
  if e!=nil{writeJSONError(w,409,"could not approve deposit");return}
  _,_=h.service.repository.db.Exec(r.Context(),"UPDATE wallet_card_receipts SET reviewer_id=$2 WHERE id=$1",id,reviewer)
  writeJSON(w,200,result);return
 }
 tx,e:=h.service.repository.db.Begin(r.Context());if e!=nil{writeJSONError(w,500,"database error");return};defer tx.Rollback(r.Context())
 tag,e:=tx.Exec(r.Context(),"UPDATE wallet_deposits SET status='failed',updated_at=now() WHERE id=$1 AND status='pending'",depositID)
 if e!=nil||tag.RowsAffected()!=1{writeJSONError(w,409,"deposit already settled");return}
 tag,e=tx.Exec(r.Context(),"UPDATE wallet_card_receipts SET status='rejected',reviewer_id=$2,reviewed_at=now() WHERE id=$1 AND status='pending_review'",id,reviewer)
 if e!=nil||tag.RowsAffected()!=1{writeJSONError(w,409,"receipt already reviewed");return}
 if tx.Commit(r.Context())!=nil{writeJSONError(w,500,"database error");return}
 writeJSON(w,200,map[string]string{"status":"rejected"})
}
