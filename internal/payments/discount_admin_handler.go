package payments

import (
 "encoding/json"
 "net/http"
 "strings"
 "time"

 "github.com/google/uuid"
 "github.com/go-chi/chi/v5"
 "backend/internal/requestcontext"
)

func (h *Handler) discountAdmin(w http.ResponseWriter,r *http.Request) bool {
 userID,ok:=requestcontext.UserID(r.Context())
 if !ok {writeJSONError(w,401,"authentication required");return false}
 var allowed bool
 err:=h.service.repository.db.QueryRow(r.Context(),"SELECT EXISTS(SELECT 1 FROM discount_admins WHERE user_id=$1)",userID).Scan(&allowed)
 if err!=nil {writeJSONError(w,500,"authorization unavailable");return false}
 if !allowed {writeJSONError(w,403,"discount administrator access required");return false}
 return true
}

type discountAdminRequest struct {
 Code string `json:"code"`
 DiscountType string `json:"discountType"`
 DiscountValue int64 `json:"discountValue"`
 Currency *string `json:"currency"`
 Active bool `json:"active"`
 StartsAt *time.Time `json:"startsAt"`
 ExpiresAt *time.Time `json:"expiresAt"`
 MaxRedemptions *int `json:"maxRedemptions"`
}

func (h *Handler) AdminListDiscounts(w http.ResponseWriter,r *http.Request){
 if !h.discountAdmin(w,r){return}
 rows,err:=h.service.repository.db.Query(r.Context(),`SELECT id,code,discount_type,discount_value,currency,active,starts_at,expires_at,max_redemptions,redemptions,created_at FROM discount_codes ORDER BY created_at DESC LIMIT 200`)
 if err!=nil {writeJSONError(w,500,"database error");return};defer rows.Close()
 result:=make([]map[string]any,0)
 for rows.Next(){
 var id uuid.UUID;var code,typ string;var value int64;var currency *string;var active bool;var starts,expires *time.Time;var max *int;var used int;var created time.Time
 if err=rows.Scan(&id,&code,&typ,&value,&currency,&active,&starts,&expires,&max,&used,&created);err!=nil {writeJSONError(w,500,"database error");return}
 result=append(result,map[string]any{"id":id,"code":code,"discountType":typ,"discountValue":value,"currency":currency,"active":active,"startsAt":starts,"expiresAt":expires,"maxRedemptions":max,"redemptions":used,"createdAt":created})
 }
 if rows.Err()!=nil {writeJSONError(w,500,"database error");return}
 writeJSON(w,200,map[string]any{"discounts":result})
}

func (h *Handler) AdminCreateDiscount(w http.ResponseWriter,r *http.Request){
 if !h.discountAdmin(w,r){return}
 var req discountAdminRequest
 dec:=json.NewDecoder(http.MaxBytesReader(w,r.Body,8192));dec.DisallowUnknownFields()
 if err:=dec.Decode(&req);err!=nil {writeJSONError(w,400,"invalid discount payload");return}
 req.Code=strings.ToUpper(strings.TrimSpace(req.Code))
 if len(req.Code)<3 || len(req.Code)>64 || req.DiscountValue<=0 {writeJSONError(w,400,"invalid discount code or value");return}
 for _,ch:=range req.Code {if !(ch>='A'&&ch<='Z'||ch>='0'&&ch<='9'||ch=='_'||ch=='-'){writeJSONError(w,400,"code must use A-Z, 0-9, underscore or hyphen");return}}
 if req.DiscountType=="percent" {
  if req.DiscountValue>10000 || req.Currency!=nil {writeJSONError(w,400,"percent discount must be at most 10000 basis points with no currency");return}
 }else if req.DiscountType=="fixed" {
  if req.Currency==nil || len(*req.Currency)!=3 {writeJSONError(w,400,"fixed discount requires currency");return}
  c:=strings.ToUpper(*req.Currency);req.Currency=&c
  for _,ch:=range c {if ch<'A'||ch>'Z' {writeJSONError(w,400,"invalid currency");return}}
 }else {writeJSONError(w,400,"invalid discount type");return}
 if req.MaxRedemptions!=nil && *req.MaxRedemptions<=0 {writeJSONError(w,400,"maxRedemptions must be positive");return}
 if req.StartsAt!=nil && req.ExpiresAt!=nil && !req.ExpiresAt.After(*req.StartsAt){writeJSONError(w,400,"expiresAt must follow startsAt");return}
 var id uuid.UUID
 err:=h.service.repository.db.QueryRow(r.Context(),`INSERT INTO discount_codes(code,discount_type,discount_value,currency,active,starts_at,expires_at,max_redemptions)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`,req.Code,req.DiscountType,req.DiscountValue,req.Currency,req.Active,req.StartsAt,req.ExpiresAt,req.MaxRedemptions).Scan(&id)
 if err!=nil {writeJSONError(w,409,"discount code already exists or could not be created");return}
 writeJSON(w,201,map[string]any{"id":id,"code":req.Code})
}

func (h *Handler) AdminDisableDiscount(w http.ResponseWriter,r *http.Request){
 if !h.discountAdmin(w,r){return}
 id,err:=uuid.Parse(chi.URLParam(r,"discountID"));if err!=nil {writeJSONError(w,400,"invalid discount ID");return}
 tag,err:=h.service.repository.db.Exec(r.Context(),"UPDATE discount_codes SET active=false WHERE id=$1",id)
 if err!=nil {writeJSONError(w,500,"database error");return}
 if tag.RowsAffected()==0 {writeJSONError(w,404,"discount not found");return}
 writeJSON(w,200,map[string]any{"id":id,"active":false})
}
