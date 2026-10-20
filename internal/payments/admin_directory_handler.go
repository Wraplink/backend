package payments

import (
 "net/http"
 "strconv"
 "strings"
 "github.com/google/uuid"
)

func adminPageLimit(r *http.Request) int {
 n,e:=strconv.Atoi(r.URL.Query().Get("limit"))
 if e!=nil||n<1{return 50}
 if n>100{return 100}
 return n
}

// AdminUsers provides a read-only, bounded user directory for access administrators.
func (h *Handler) AdminUsers(w http.ResponseWriter,r *http.Request){
 if _,ok:=h.requireAccessAdmin(w,r);!ok{return}
 search:=strings.TrimSpace(r.URL.Query().Get("search"))
 if len(search)>200 {writeJSONError(w,400,"search too long");return}
 rows,err:=h.service.repository.db.Query(r.Context(),`SELECT id,email,full_name,account_status,email_verified,created_at
 FROM users WHERE ($1='' OR email ILIKE '%'||$1||'%' OR full_name ILIKE '%'||$1||'%')
 ORDER BY created_at DESC,id DESC LIMIT $2`,search,adminPageLimit(r))
 if err!=nil{writeJSONError(w,500,"database error");return};defer rows.Close()
 items:=make([]map[string]any,0)
 for rows.Next(){
  var id uuid.UUID;var email,name,status string;var verified bool;var created any
  if err=rows.Scan(&id,&email,&name,&status,&verified,&created);err!=nil{writeJSONError(w,500,"database error");return}
  items=append(items,map[string]any{"id":id,"email":email,"name":name,"status":status,"emailVerified":verified,"createdAt":created})
 }
 if rows.Err()!=nil{writeJSONError(w,500,"database error");return}
 writeJSON(w,200,map[string]any{"users":items})
}

// AdminOrders returns subscription and payment state without exposing credentials or receipts.
func (h *Handler) AdminOrders(w http.ResponseWriter,r *http.Request){
 if _,ok:=h.requireAccessAdmin(w,r);!ok{return}
 rows,err:=h.service.repository.db.Query(r.Context(),`SELECT s.id,s.user_id,u.email,s.status,s.price_minor,s.currency,s.created_at,
 COALESCE((SELECT c.status FROM card_transfer_receipts c WHERE c.subscription_id=s.id),'')
 FROM user_subscriptions s JOIN users u ON u.id=s.user_id
 ORDER BY s.created_at DESC,s.id DESC LIMIT $1`,adminPageLimit(r))
 if err!=nil{writeJSONError(w,500,"database error");return};defer rows.Close()
 items:=make([]map[string]any,0)
 for rows.Next(){
  var id,user uuid.UUID;var email,status,currency,receipt string;var amount int64;var created any
  if err=rows.Scan(&id,&user,&email,&status,&amount,&currency,&created,&receipt);err!=nil{writeJSONError(w,500,"database error");return}
  items=append(items,map[string]any{"id":id,"userId":user,"email":email,"status":status,"amountMinor":amount,"currency":currency,"receiptStatus":receipt,"createdAt":created})
 }
 if rows.Err()!=nil{writeJSONError(w,500,"database error");return}
 writeJSON(w,200,map[string]any{"orders":items})
}
