package payments

import (
 "encoding/json"
 "net/http"
 "log/slog"

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
 tx,err:=h.service.repository.db.Begin(r.Context());if err!=nil{slog.Error("admin user status database failure","error",err,"target_user_id",target,"action",body.Action);writeJSONError(w,500,"database error");return}
 defer tx.Rollback(r.Context())
 var previous string
 err=tx.QueryRow(r.Context(),"SELECT account_status FROM users WHERE id=$1 FOR UPDATE",target).Scan(&previous)
 if err==pgx.ErrNoRows{writeJSONError(w,404,"user not found");return}
 if err!=nil{slog.Error("admin user status database failure","error",err,"target_user_id",target,"action",body.Action);writeJSONError(w,500,"database error");return}
 if previous=="deleted"{writeJSONError(w,409,"deleted account cannot be modified");return}
 if body.Action=="restore"&&previous!="suspended"{writeJSONError(w,409,"only suspended accounts can be restored");return}
 // Protect every access administrator, including the last one, from accidental removal.
 var admin bool
 if err=tx.QueryRow(r.Context(),"SELECT EXISTS(SELECT 1 FROM access_admins WHERE user_id=$1)",target).Scan(&admin);err!=nil{slog.Error("admin user status database failure","error",err,"target_user_id",target,"action",body.Action);writeJSONError(w,500,"database error");return}
 if admin{writeJSONError(w,403,"access administrator accounts cannot be modified here");return}
 if body.Action=="delete" {
  // Delete non-cascading references first. The remaining user-owned tables
  // have ON DELETE CASCADE; all changes occur in the same transaction.
  statements:=[]string{
   "DELETE FROM admin_permission_events WHERE actor_id=$1 OR target_id=$1",
   "DELETE FROM payment_review_events WHERE reviewer_id=$1 OR receipt_id IN (SELECT id FROM card_transfer_receipts WHERE user_id=$1 OR subscription_id IN (SELECT id FROM user_subscriptions WHERE user_id=$1))",
   "DELETE FROM card_transfer_receipts WHERE user_id=$1 OR subscription_id IN (SELECT id FROM user_subscriptions WHERE user_id=$1)",
   "DELETE FROM discount_redemptions WHERE user_id=$1 OR subscription_id IN (SELECT id FROM user_subscriptions WHERE user_id=$1)",
   "DELETE FROM security_events WHERE user_id=$1",
   "DELETE FROM users WHERE id=$1",
  }
  for _,statement:=range statements {
   if _,err=tx.Exec(r.Context(),statement,target);err!=nil{
    slog.Error("admin user deletion failed","error",err,"target_user_id",target,"sql",statement)
    writeJSONError(w,500,"database error");return
   }
  }
  // Audit deletion without a user FK because the user record no longer exists.
  if _,err=tx.Exec(r.Context(),"INSERT INTO security_events(event_type,metadata) VALUES($1,jsonb_build_object('actorId',$2::text,'deletedUserId',$3::text,'previousStatus',$4::text))",event,actor.String(),target.String(),previous);err!=nil{
   slog.Error("admin user deletion audit failure","error",err,"target_user_id",target)
   writeJSONError(w,500,"audit write failed");return
  }
  if err=tx.Commit(r.Context());err!=nil{
   slog.Error("admin user deletion commit failure","error",err,"target_user_id",target)
   writeJSONError(w,500,"database error");return
  }
  writeJSON(w,200,map[string]any{"userId":target,"status":"deleted"})
  return
 }
 if _,err=tx.Exec(r.Context(),"UPDATE users SET account_status=$2,updated_at=now() WHERE id=$1",target,status);err!=nil{slog.Error("admin user status database failure","error",err,"target_user_id",target,"action",body.Action);writeJSONError(w,500,"database error");return}
 if _,err=tx.Exec(r.Context(),"UPDATE user_sessions SET revoked_at=COALESCE(revoked_at,now()),revoked_reason='admin_account_action' WHERE user_id=$1 AND revoked_at IS NULL",target);err!=nil{slog.Error("admin user status database failure","error",err,"target_user_id",target,"action",body.Action);writeJSONError(w,500,"database error");return}
 if _,err=tx.Exec(r.Context(),"INSERT INTO security_events(user_id,event_type,metadata) VALUES($1,$2,jsonb_build_object('actorId',$3::text,'previousStatus',$4::text))",target,event,actor.String(),previous);err!=nil{slog.Error("admin user status audit failure","error",err,"target_user_id",target,"action",body.Action);writeJSONError(w,500,"audit write failed");return}
 if err=tx.Commit(r.Context());err!=nil{slog.Error("admin user status database failure","error",err,"target_user_id",target,"action",body.Action);writeJSONError(w,500,"database error");return}
 writeJSON(w,200,map[string]any{"userId":target,"status":status})
}
