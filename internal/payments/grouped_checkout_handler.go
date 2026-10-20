package payments

import (
 "encoding/json"
 "io"
 "net/http"
 "strings"
 "github.com/google/uuid"
 "backend/internal/requestcontext"
)

func (h *Handler) SubmitGroupedCardReceipt(w http.ResponseWriter,r *http.Request) {
 userID,ok:=requestcontext.UserID(r.Context());if !ok{writeJSONError(w,401,"authentication required");return}
 r.Body=http.MaxBytesReader(w,r.Body,6<<20)
 if r.ParseMultipartForm(6<<20)!=nil{writeJSONError(w,400,"invalid upload");return}
 file,header,err:=r.FormFile("receipt");if err!=nil{writeJSONError(w,400,"receipt required");return};defer file.Close()
 b,err:=io.ReadAll(io.LimitReader(file,(5<<20)+1));if err!=nil||len(b)==0||len(b)>5<<20{writeJSONError(w,400,"invalid receipt size");return}
 typ:=http.DetectContentType(b)
 if typ!="image/jpeg"&&typ!="image/png"&&typ!="image/webp"&&typ!="application/pdf"{writeJSONError(w,400,"unsupported receipt type");return}
 cardID,err:=uuid.Parse(strings.TrimSpace(r.FormValue("paymentCardId")));if err!=nil{writeJSONError(w,400,"invalid destination card");return}
 var ids []uuid.UUID
 if json.Unmarshal([]byte(r.FormValue("subscriptionIds")),&ids)!=nil||len(ids)==0||len(ids)>100{writeJSONError(w,400,"invalid subscription list");return}
 seen:=map[uuid.UUID]bool{}
 for _,id:=range ids{if id==uuid.Nil||seen[id]{writeJSONError(w,400,"duplicate or invalid subscription");return};seen[id]=true}
 tx,err:=h.service.repository.db.Begin(r.Context());if err!=nil{writeJSONError(w,500,"database error");return};defer tx.Rollback(r.Context())
 var active bool
 if tx.QueryRow(r.Context(),"SELECT active FROM payment_cards WHERE id=$1",cardID).Scan(&active)!=nil||!active{writeJSONError(w,400,"destination card unavailable");return}
 // Lock in stable order to avoid deadlocks during concurrent submissions.
 ordered:=append([]uuid.UUID(nil),ids...)
 for i:=0;i<len(ordered);i++{for j:=i+1;j<len(ordered);j++{if ordered[j].String()<ordered[i].String(){ordered[i],ordered[j]=ordered[j],ordered[i]}}}
 var total int64;var currency string
 for _,id:=range ordered{
  var amount int64;var cur string
  err=tx.QueryRow(r.Context(),`SELECT price_minor,currency FROM user_subscriptions WHERE id=$1 AND user_id=$2 AND status='pending_payment' FOR UPDATE`,id,userID).Scan(&amount,&cur)
  if err!=nil{writeJSONError(w,409,"one or more orders are not pending");return}
  var hasReceipt bool
  if tx.QueryRow(r.Context(),"SELECT EXISTS(SELECT 1 FROM card_transfer_receipts WHERE subscription_id=$1)",id).Scan(&hasReceipt)!=nil||hasReceipt{writeJSONError(w,409,"one or more orders already have receipts");return}
  if currency!=""&&currency!=cur{writeJSONError(w,400,"mixed currencies are not supported");return}
  currency=cur
  if amount<0||total>int64(^uint64(0)>>1)-amount{writeJSONError(w,400,"amount overflow");return}
  total+=amount
 }
 groupID:=uuid.New()
 for _,id:=range ids{
  var amount int64
  if tx.QueryRow(r.Context(),"SELECT price_minor FROM user_subscriptions WHERE id=$1",id).Scan(&amount)!=nil{writeJSONError(w,500,"database error");return}
  _,err=tx.Exec(r.Context(),`INSERT INTO card_transfer_receipts(user_id,subscription_id,filename,content_type,receipt_bytes,payable_minor,payment_card_id,payment_group_id)
  VALUES($1,$2,$3,$4,$5,$6,$7,$8)`,userID,id,header.Filename,typ,b,amount,cardID,groupID)
  if err!=nil{writeJSONError(w,409,"could not save grouped receipt");return}
 }
 if tx.Commit(r.Context())!=nil{writeJSONError(w,500,"database error");return}
 writeJSON(w,201,map[string]any{"id":groupID,"status":"pending_review","orderCount":len(ids),"payableMinor":total,"currency":currency})
}
