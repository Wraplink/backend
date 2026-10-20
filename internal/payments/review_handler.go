package payments

import (
 "encoding/json"
 "errors"
 "time"
 "net/http"
 "github.com/go-chi/chi/v5"
 "github.com/google/uuid"
 "github.com/jackc/pgx/v5"
 "backend/internal/requestcontext"
)

func (h *Handler) reviewAdmin(w http.ResponseWriter,r *http.Request)(uuid.UUID,bool){
 userID,ok:=requestcontext.UserID(r.Context())
 if !ok {writeJSONError(w,401,"authentication required");return uuid.Nil,false}
 var allowed bool
 err:=h.service.repository.db.QueryRow(r.Context(),"SELECT EXISTS(SELECT 1 FROM payment_review_admins WHERE user_id=$1)",userID).Scan(&allowed)
 if err!=nil {writeJSONError(w,500,"authorization unavailable");return uuid.Nil,false}
 if !allowed {writeJSONError(w,403,"payment reviewer access required");return uuid.Nil,false}
 return userID,true
}

func (h *Handler) PendingReceipts(w http.ResponseWriter,r *http.Request){
 _,ok:=h.reviewAdmin(w,r);if !ok{return}
 rows,err:=h.service.repository.db.Query(r.Context(),`SELECT c.id,c.subscription_id,c.user_id,c.filename,c.content_type,c.payable_minor,s.currency,c.created_at,c.payment_group_id
 FROM card_transfer_receipts c JOIN user_subscriptions s ON s.id=c.subscription_id
 WHERE c.status='pending_review' AND (c.payment_group_id IS NULL OR c.id=(SELECT x.id FROM card_transfer_receipts x WHERE x.payment_group_id=c.payment_group_id ORDER BY x.id LIMIT 1)) ORDER BY c.created_at LIMIT 100`)
 if err!=nil {writeJSONError(w,500,"could not list receipts");return};defer rows.Close()
 items:=make([]map[string]any,0)
 for rows.Next(){
 var id,sub,user uuid.UUID;var name,typ,currency string;var payable int64;var created time.Time;var groupID *uuid.UUID
 if err=rows.Scan(&id,&sub,&user,&name,&typ,&payable,&currency,&created,&groupID);err!=nil {writeJSONError(w,500,"could not read receipts");return}
 count:=1
 if groupID!=nil {
  if err=h.service.repository.db.QueryRow(r.Context(),"SELECT count(*),sum(payable_minor) FROM card_transfer_receipts WHERE payment_group_id=$1",*groupID).Scan(&count,&payable);err!=nil{writeJSONError(w,500,"could not calculate group total");return}
 }
 items=append(items,map[string]any{"id":id,"subscriptionId":sub,"userId":user,"filename":name,"contentType":typ,"payableMinor":payable,"currency":currency,"createdAt":created,"orderCount":count,"paymentGroupId":groupID})
 }
 if rows.Err()!=nil {writeJSONError(w,500,"could not read receipts");return}
 writeJSON(w,200,map[string]any{"receipts":items})
}

func (h *Handler) ReceiptAttachment(w http.ResponseWriter,r *http.Request){
 _,ok:=h.reviewAdmin(w,r);if !ok{return}
 id,err:=uuid.Parse(chi.URLParam(r,"receiptID"));if err!=nil {writeJSONError(w,400,"invalid receipt ID");return}
 var data []byte;var contentType string
 err=h.service.repository.db.QueryRow(r.Context(),"SELECT receipt_bytes,content_type FROM card_transfer_receipts WHERE id=$1",id).Scan(&data,&contentType)
 if errors.Is(err,pgx.ErrNoRows){writeJSONError(w,404,"receipt not found");return}
 if err!=nil {writeJSONError(w,500,"could not read receipt");return}
 w.Header().Set("Content-Type",contentType)
 w.Header().Set("Content-Disposition","attachment; filename=receipt")
 w.Header().Set("Cache-Control","no-store")
 w.Header().Set("X-Content-Type-Options","nosniff")
 w.WriteHeader(200);_,_=w.Write(data)
}

func (h *Handler) ReviewReceipt(w http.ResponseWriter,r *http.Request){
 reviewer,ok:=h.reviewAdmin(w,r);if !ok{return}
 id,err:=uuid.Parse(chi.URLParam(r,"receiptID"));if err!=nil {writeJSONError(w,400,"invalid receipt ID");return}
 var body struct{Decision string `json:"decision"`}
 if err=json.NewDecoder(http.MaxBytesReader(w,r.Body,2048)).Decode(&body);err!=nil || (body.Decision!="approved" && body.Decision!="rejected"){writeJSONError(w,400,"decision must be approved or rejected");return}
 tx,err:=h.service.repository.db.Begin(r.Context());if err!=nil {writeJSONError(w,500,"database error");return};defer tx.Rollback(r.Context())
 var sub,user uuid.UUID;var discount *uuid.UUID;var amount int64;var currency string;var groupID *uuid.UUID
 err=tx.QueryRow(r.Context(),`SELECT c.subscription_id,c.user_id,c.discount_id,c.payable_minor,s.currency,c.payment_group_id
 FROM card_transfer_receipts c JOIN user_subscriptions s ON s.id=c.subscription_id
 WHERE c.id=$1 AND c.status='pending_review' AND s.status='pending_payment' FOR UPDATE OF c,s`,id).Scan(&sub,&user,&discount,&amount,&currency,&groupID)
 if errors.Is(err,pgx.ErrNoRows){writeJSONError(w,409,"receipt already reviewed or subscription not pending");return}
 if err!=nil {writeJSONError(w,500,"database error");return}
 if groupID!=nil {
  // A single review decision applies atomically to every order sharing this receipt.
  var lockedUser uuid.UUID
  if tx.QueryRow(r.Context(),"SELECT id FROM users WHERE id=$1 FOR UPDATE",user).Scan(&lockedUser)!=nil{writeJSONError(w,500,"database error");return}
  rows,e:=tx.Query(r.Context(),`SELECT c.id,c.subscription_id,c.payable_minor,s.currency
   FROM card_transfer_receipts c JOIN user_subscriptions s ON s.id=c.subscription_id
   WHERE c.payment_group_id=$1 AND c.user_id=$2 AND c.status='pending_review' AND s.status='pending_payment'
   ORDER BY c.subscription_id FOR UPDATE OF c,s`,*groupID,user)
  if e!=nil{writeJSONError(w,500,"could not lock grouped orders");return}
  type entry struct{id,sub uuid.UUID;amount int64;currency string}
  entries:=[]entry{}
  for rows.Next(){var v entry;if rows.Scan(&v.id,&v.sub,&v.amount,&v.currency)!=nil{rows.Close();writeJSONError(w,500,"could not read grouped orders");return};entries=append(entries,v)}
  readErr:=rows.Err();rows.Close()
  if readErr!=nil||len(entries)==0{writeJSONError(w,409,"group already reviewed");return}
  var groupCount int
  if tx.QueryRow(r.Context(),"SELECT count(*) FROM card_transfer_receipts WHERE payment_group_id=$1",*groupID).Scan(&groupCount)!=nil||groupCount!=len(entries){writeJSONError(w,409,"group changed; refresh");return}
  if body.Decision=="approved" {
   if _,e=tx.Exec(r.Context(),"SELECT promote_user_subscription($1)",user);e!=nil{writeJSONError(w,500,"queue reconciliation failed");return}
  }
  for _,v:=range entries{
   if body.Decision=="approved" {
    _,e=tx.Exec(r.Context(),`INSERT INTO payments(user_id,subscription_id,method,amount_minor,currency,status,idempotency_key,completed_at)
    VALUES($1,$2,'card_to_card',$3,$4,'succeeded',$5,now())`,user,v.sub,v.amount,v.currency,"card-receipt:"+v.id.String())
    if e!=nil{writeJSONError(w,409,"payment already recorded");return}
    _,e=tx.Exec(r.Context(),"UPDATE user_subscriptions SET status='confirmed',updated_at=now() WHERE id=$1",v.sub)
    if e!=nil{writeJSONError(w,500,"could not confirm order");return}
   }
   if body.Decision=="rejected" {if _,e=tx.Exec(r.Context(),"UPDATE user_subscriptions SET status='cancelled',cancelled_at=now(),updated_at=now() WHERE id=$1 AND status='pending_payment'",v.sub);e!=nil{writeJSONError(w,500,"could not reject order");return}}
   _,e=tx.Exec(r.Context(),"UPDATE card_transfer_receipts SET status=$2,reviewed_at=now() WHERE id=$1",v.id,body.Decision)
   if e!=nil{writeJSONError(w,500,"could not review receipt");return}
   _,e=tx.Exec(r.Context(),"INSERT INTO payment_review_events(receipt_id,reviewer_id,decision) VALUES($1,$2,$3)",v.id,reviewer,body.Decision)
   if e!=nil{writeJSONError(w,500,"could not log review");return}
  }
  if body.Decision=="approved" {
   if _,e=tx.Exec(r.Context(),"SELECT promote_user_subscription($1)",user);e!=nil{writeJSONError(w,500,"could not activate queue");return}
  }
  if tx.Commit(r.Context())!=nil{writeJSONError(w,500,"could not commit review");return}
  writeJSON(w,200,map[string]any{"id":*groupID,"status":body.Decision,"orderCount":len(entries)})
  return
 }
 if body.Decision=="approved" {
  // Serialize approvals for different subscriptions belonging to the same user.
  // This lock prevents two reviewers from simultaneously activating two plans.
  var lockedUser uuid.UUID
  err=tx.QueryRow(r.Context(),"SELECT id FROM users WHERE id=$1 FOR UPDATE",user).Scan(&lockedUser)
  if err!=nil {writeJSONError(w,500,"database error");return}
  // Reconcile expired/exhausted plans and activate the oldest confirmed order.
  if _,err=tx.Exec(r.Context(),"SELECT promote_user_subscription($1)",user);err!=nil {writeJSONError(w,500,"subscription reconciliation failed");return}
  _,err=tx.Exec(r.Context(),`INSERT INTO payments(user_id,subscription_id,method,amount_minor,currency,status,idempotency_key,completed_at)
 VALUES($1,$2,'card_to_card',$3,$4,'succeeded',$5,now())`,user,sub,amount,currency,"card-receipt:"+id.String())
  if err!=nil {writeJSONError(w,409,"payment already recorded");return}
  // Approval reserves a paid plan; only the first available plan becomes active.
  tag,e:=tx.Exec(r.Context(),`UPDATE user_subscriptions SET status='confirmed',updated_at=now()
 WHERE id=$1 AND status='pending_payment'`,sub)
  if e!=nil || tag.RowsAffected()!=1 {writeJSONError(w,409,"subscription could not be confirmed");return}
  if _,err=tx.Exec(r.Context(),"SELECT promote_user_subscription($1)",user);err!=nil {writeJSONError(w,500,"subscription activation failed");return}
 }else {
  if _,err=tx.Exec(r.Context(),"UPDATE user_subscriptions SET status='cancelled',cancelled_at=now(),updated_at=now() WHERE id=$1 AND status='pending_payment'",sub);err!=nil{writeJSONError(w,500,"could not reject order");return}
  if discount!=nil {
  _,err=tx.Exec(r.Context(),"DELETE FROM discount_redemptions WHERE discount_id=$1 AND subscription_id=$2",*discount,sub)
  if err!=nil {writeJSONError(w,500,"database error");return}
  _,err=tx.Exec(r.Context(),"UPDATE discount_codes SET redemptions=GREATEST(0,redemptions-1) WHERE id=$1",*discount)
  if err!=nil {writeJSONError(w,500,"database error");return}
  }
 }
 tag,err:=tx.Exec(r.Context(),"UPDATE card_transfer_receipts SET status=$2,reviewed_at=now() WHERE id=$1 AND status='pending_review'",id,body.Decision)
 if err!=nil || tag.RowsAffected()!=1 {writeJSONError(w,409,"receipt could not be reviewed");return}
 _,err=tx.Exec(r.Context(),"INSERT INTO payment_review_events(receipt_id,reviewer_id,decision) VALUES($1,$2,$3)",id,reviewer,body.Decision)
 if err!=nil {writeJSONError(w,500,"could not record review");return}
 if err=tx.Commit(r.Context());err!=nil {writeJSONError(w,500,"could not finalize review");return}
 writeJSON(w,200,map[string]any{"id":id,"status":body.Decision})
}
