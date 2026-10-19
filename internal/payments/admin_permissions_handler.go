package payments

import (
 "encoding/json"
 "net/http"
 "github.com/go-chi/chi/v5"
 "github.com/google/uuid"
 "backend/internal/requestcontext"
)

func (h *Handler) requireAccessAdmin(w http.ResponseWriter,r *http.Request)(uuid.UUID,bool){
 actor,ok:=requestcontext.UserID(r.Context())
 if !ok {writeJSONError(w,401,"authentication required");return uuid.Nil,false}
 var allowed bool
 if err:=h.service.repository.db.QueryRow(r.Context(),"SELECT EXISTS(SELECT 1 FROM access_admins WHERE user_id=$1)",actor).Scan(&allowed);err!=nil{
  writeJSONError(w,500,"authorization unavailable");return uuid.Nil,false
 }
 if !allowed {writeJSONError(w,403,"access administrator required");return uuid.Nil,false}
 return actor,true
}

func (h *Handler) ListAdminPermissions(w http.ResponseWriter,r *http.Request){
 if _,ok:=h.requireAccessAdmin(w,r);!ok{return}
 rows,err:=h.service.repository.db.Query(r.Context(),`SELECT u.id,u.email,
 EXISTS(SELECT 1 FROM payment_review_admins p WHERE p.user_id=u.id),
 EXISTS(SELECT 1 FROM discount_admins d WHERE d.user_id=u.id)
 FROM users u WHERE EXISTS(SELECT 1 FROM payment_review_admins p WHERE p.user_id=u.id)
 OR EXISTS(SELECT 1 FROM discount_admins d WHERE d.user_id=u.id)
 ORDER BY u.email LIMIT 500`)
 if err!=nil {writeJSONError(w,500,"database error");return};defer rows.Close()
 result:=make([]map[string]any,0)
 for rows.Next(){
  var id uuid.UUID;var email string;var reviewer,discount bool
  if err=rows.Scan(&id,&email,&reviewer,&discount);err!=nil{writeJSONError(w,500,"database error");return}
  result=append(result,map[string]any{"userId":id,"email":email,"paymentReview":reviewer,"discountAdmin":discount})
 }
 if rows.Err()!=nil{writeJSONError(w,500,"database error");return}
 writeJSON(w,200,map[string]any{"users":result})
}

func (h *Handler) UpdateAdminPermission(w http.ResponseWriter,r *http.Request){
 actor,ok:=h.requireAccessAdmin(w,r);if !ok{return}
 target,err:=uuid.Parse(chi.URLParam(r,"userID"))
 if err!=nil {writeJSONError(w,400,"invalid user ID");return}
 permission:=chi.URLParam(r,"permission")
 var table string
 switch permission {
 case "payment_review":table="payment_review_admins"
 case "discount_admin":table="discount_admins"
 default:writeJSONError(w,400,"invalid permission");return
 }
 var body struct{Enabled *bool `json:"enabled"`}
 decoder:=json.NewDecoder(http.MaxBytesReader(w,r.Body,1024));decoder.DisallowUnknownFields()
 if err=decoder.Decode(&body);err!=nil||body.Enabled==nil{writeJSONError(w,400,"enabled boolean required");return}
 tx,err:=h.service.repository.db.Begin(r.Context())
 if err!=nil{writeJSONError(w,500,"database error");return}
 defer tx.Rollback(r.Context())
 var exists bool
 err=tx.QueryRow(r.Context(),"SELECT EXISTS(SELECT 1 FROM users WHERE id=$1)",target).Scan(&exists)
 if err!=nil{writeJSONError(w,500,"database error");return}
 if !exists{writeJSONError(w,404,"user not found");return}
 if *body.Enabled{
  _,err=tx.Exec(r.Context(),"INSERT INTO "+table+"(user_id) VALUES($1) ON CONFLICT DO NOTHING",target)
 }else{
  _,err=tx.Exec(r.Context(),"DELETE FROM "+table+" WHERE user_id=$1",target)
 }
 if err!=nil{writeJSONError(w,500,"database error");return}
 _,err=tx.Exec(r.Context(),"INSERT INTO admin_permission_events(actor_id,target_id,permission,enabled) VALUES($1,$2,$3,$4)",actor,target,permission,*body.Enabled)
 if err!=nil{writeJSONError(w,500,"audit write failed");return}
 if err=tx.Commit(r.Context());err!=nil{writeJSONError(w,500,"database error");return}
 writeJSON(w,200,map[string]any{"userId":target,"permission":permission,"enabled":*body.Enabled})
}
