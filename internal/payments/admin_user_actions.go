package payments

import (
 "encoding/json"
 "net/http"

 "github.com/go-chi/chi/v5"
 "github.com/google/uuid"
 "github.com/jackc/pgx/v5"
)

// AdminUpdateUserStatus suspends, restores, or soft-deletes a user.
// Soft deletion preserves billing and audit references and cannot be undone here.
func (h *Handler) AdminUpdateUserStatus(w http.ResponseWriter,r *http.Request) {
 actor,ok:=h.requireAccessAdmin(w,r);if !ok{return}
 target,err:=uuid.Parse(chi.URLParam(r,"userID"));if err!=nil{writeJSONError(w,400,"invalid user ID");return}
 if target==actor{writeJSONError(w,403,"cannot modify your own account");return}
 var body struct{Action string `json:"action"`}
 decoder:=json.NewDecoder(http.MaxBytesReader(w,r.Body,1024));decoder.DisallowUnknownFields()
 if err=decoder.Decode(&body);err!=nil{writeJSONError(w,400,"invalid request");return}
 var status,event string
 switch body.Action {
 case "suspend":status="suspended";event="ADMIN_USER_SUSPENDED"
 case "restore":status="active";event="ADMIN_USER_RESTORED"
 case "delete":status="deleted";event="ADMIN_USER_DELETED"
 default:writeJSONError(w,400,"invalid action");return
 }
 tx,err:=h.service.repository.db.Begin(r.Context());if err!=nil{writeJSONError(w,500,"database error");return}
 defer tx.Rollback(r.Context())
 var previous string
 err=tx.QueryRow(r.Context(),"SELECT account_status FROM users WHERE id=$1 FOR UPDATE",target).Scan(&previous)
 if err==pgx.ErrNoRows{writeJSONError(w,404,"user not found");return}
 if err!=nil{writeJSONError(w,500,"database error");return}
 if previous=="deleted"{writeJSONError(w,409,"deleted account cannot be modified");return}
 if body.Action=="restore"&&previous!="suspended"{writeJSONError(w,409,"only suspended accounts can be restored");return}
 // Protect every access administrator, including the last one, from accidental removal.
 var admin bool
 if err=tx.QueryRow(r.Context(),"SELECT EXISTS(SELECT 1 FROM access_admins WHERE user_id=$1)",target).Scan(&admin);err!=nil{writeJSONError(w,500,"database error");return}
 if admin{writeJSONError(w,403,"access administrator accounts cannot be modified here");return}
 if _,err=tx.Exec(r.Context(),"UPDATE users SET account_status=$2,updated_at=now() WHERE id=$1",target,status);err!=nil{writeJSONError(w,500,"database error");return}
 if _,err=tx.Exec(r.Context(),"UPDATE user_sessions SET revoked_at=COALESCE(revoked_at,now()),revoked_reason='admin_account_action' WHERE user_id=$1 AND revoked_at IS NULL",target);err!=nil{writeJSONError(w,500,"database error");return}
 if _,err=tx.Exec(r.Context(),"INSERT INTO security_events(user_id,event_type,metadata) VALUES($1,$2,jsonb_build_object('actorId',$3::text,'previousStatus',$4))",target,event,actor.String(),previous);err!=nil{writeJSONError(w,500,"audit write failed");return}
 if err=tx.Commit(r.Context());err!=nil{writeJSONError(w,500,"database error");return}
 writeJSON(w,200,map[string]any{"userId":target,"status":status})
}
