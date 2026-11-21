package publicip

import (
 "encoding/json"
 "errors"
 "io"
 "net/http"
 "net/netip"
 "strings"

 "github.com/go-chi/chi/v5"
 "github.com/google/uuid"
)

// Handler is deliberately transport-only. The application's authentication
// middleware must supply a verified user ID through AuthenticatedUserID.
// Never accept a user ID from a request body, query, or arbitrary header.
type Handler struct {
 Store Store
 AuthenticatedUserID func(*http.Request) (uuid.UUID, bool)
 TrustedProxyPrefixes []netip.Prefix
 ObservedClientIP func(*http.Request) (string,error)
}

// Routes returns a router for mounting below /api/v1/me/public-ips.
// The parent MUST apply the application's existing authentication middleware.
func (h Handler) Routes() http.Handler {
 r:=chi.NewRouter()
 r.Get("/",h.list)
 r.Post("/",h.add)
 r.Get("/history",h.history)
 r.Get("/observed",h.observed)
 r.Get("/subscriptions/{subscriptionID}/sync",h.syncStatus)
 r.Post("/subscriptions/{subscriptionID}/sync/retry",h.retrySync)
 r.Patch("/{id}",h.rename)
 r.Delete("/{id}",h.remove)
 r.Post("/{id}/verify",h.verify)
 return r
}

func (h Handler) user(w http.ResponseWriter,r *http.Request)(uuid.UUID,bool){
 if h.AuthenticatedUserID==nil {http.Error(w,"authentication unavailable",http.StatusInternalServerError);return uuid.Nil,false}
 id,ok:=h.AuthenticatedUserID(r)
 if !ok || id==uuid.Nil {http.Error(w,"unauthorized",http.StatusUnauthorized);return uuid.Nil,false}
 return id,true
}
func respond(w http.ResponseWriter,status int,value any){
 w.Header().Set("Content-Type","application/json")
 w.WriteHeader(status)
 _=json.NewEncoder(w).Encode(value)
}
func decode(w http.ResponseWriter,r *http.Request,v any)bool{
 r.Body=http.MaxBytesReader(w,r.Body,4096)
 dec:=json.NewDecoder(r.Body)
 dec.DisallowUnknownFields()
 if err:=dec.Decode(v);err!=nil{http.Error(w,"invalid JSON",http.StatusBadRequest);return false}
 var extra any
 if err:=dec.Decode(&extra);!errors.Is(err,io.EOF){http.Error(w,"unexpected trailing JSON",http.StatusBadRequest);return false}
 return true
}
func pathID(w http.ResponseWriter,r *http.Request)(uuid.UUID,bool){
 id,err:=uuid.Parse(chi.URLParam(r,"id"))
 if err!=nil{http.Error(w,"invalid IP ID",http.StatusBadRequest);return uuid.Nil,false}
 return id,true
}
func writeError(w http.ResponseWriter,err error){
 switch{
 case errors.Is(err,ErrNotOwned):http.Error(w,"IP not found",http.StatusNotFound)
 case errors.Is(err,ErrActiveAddress):http.Error(w,err.Error(),http.StatusConflict)
 case errors.Is(err,ErrDuplicateAddress):http.Error(w,err.Error(),http.StatusConflict)
 case errors.Is(err,ErrInvalidPublicIPv4),errors.Is(err,ErrInvalidLabel),errors.Is(err,ErrAddressMismatch):http.Error(w,err.Error(),http.StatusBadRequest)
 default:http.Error(w,"internal error",http.StatusInternalServerError)
 }
}
func (h Handler) list(w http.ResponseWriter,r *http.Request){
 user,ok:=h.user(w,r);if !ok{return}
 addresses,err:=h.Store.List(r.Context(),user);if err!=nil{writeError(w,err);return}
 respond(w,http.StatusOK,map[string]any{"items":addresses})
}
func (h Handler) add(w http.ResponseWriter,r *http.Request){
 user,ok:=h.user(w,r);if !ok{return}
 var request struct{IP string `json:"ipAddress"`;Label string `json:"label"`}
 if !decode(w,r,&request){return}
 id,err:=h.Store.Add(r.Context(),user,request.IP,request.Label)
 if err!=nil{writeError(w,err);return}
 respond(w,http.StatusCreated,map[string]any{"id":id})
}
func (h Handler) rename(w http.ResponseWriter,r *http.Request){
 user,ok:=h.user(w,r);if !ok{return}
 id,ok:=pathID(w,r);if !ok{return}
 var request struct{Label string `json:"label"`}
 if !decode(w,r,&request){return}
 if err:=h.Store.Rename(r.Context(),user,id,request.Label);err!=nil{writeError(w,err);return}
 w.WriteHeader(http.StatusNoContent)
}
func (h Handler) remove(w http.ResponseWriter,r *http.Request){
 user,ok:=h.user(w,r);if !ok{return}
 id,ok:=pathID(w,r);if !ok{return}
 if err:=h.Store.Remove(r.Context(),user,id);err!=nil{writeError(w,err);return}
 w.WriteHeader(http.StatusNoContent)
}
func (h Handler) history(w http.ResponseWriter,r *http.Request){
 user,ok:=h.user(w,r);if !ok{return}
 items,err:=h.Store.History(r.Context(),user,50)
 if err!=nil{writeError(w,err);return}
 respond(w,http.StatusOK,map[string]any{"items":items})
}
func (h Handler) clientIP(r *http.Request)(string,error){
 if h.ObservedClientIP!=nil{return h.ObservedClientIP(r)}
 return TrustedClientIPv4(r,h.TrustedProxyPrefixes)
}
func (h Handler) observed(w http.ResponseWriter,r *http.Request){
 if _,ok:=h.user(w,r);!ok{return}
 ip,err:=h.clientIP(r)
 if err!=nil{http.Error(w,"client public IPv4 unavailable",http.StatusUnprocessableEntity);return}
 respond(w,http.StatusOK,map[string]string{"ipAddress":ip})
}
func (h Handler) verify(w http.ResponseWriter,r *http.Request){
 user,ok:=h.user(w,r);if !ok{return}
 id,ok:=pathID(w,r);if !ok{return}
 ip,err:=h.clientIP(r)
 if err!=nil{http.Error(w,"client public IPv4 unavailable",http.StatusUnprocessableEntity);return}
 if err=h.Store.VerifyObserved(r.Context(),user,id,strings.TrimSpace(ip));err!=nil{writeError(w,err);return}
 w.WriteHeader(http.StatusNoContent)
}

func (h Handler) syncStatus(w http.ResponseWriter,r *http.Request){
 user,ok:=h.user(w,r);if !ok{return}
 subscriptionID,err:=uuid.Parse(chi.URLParam(r,"subscriptionID"))
 if err!=nil {http.Error(w,"invalid subscription ID",http.StatusBadRequest);return}
 status,err:=h.Store.GetSyncStatus(r.Context(),user,subscriptionID)
 if err!=nil {
  if errors.Is(err,ErrSubscriptionNotOwned) {http.Error(w,"subscription not found",http.StatusNotFound);return}
  writeError(w,err);return
 }
 respond(w,http.StatusOK,status)
}

func (h Handler) retrySync(w http.ResponseWriter,r *http.Request){
 user,ok:=h.user(w,r);if !ok{return}
 subscriptionID,err:=uuid.Parse(chi.URLParam(r,"subscriptionID"))
 if err!=nil{http.Error(w,"invalid subscription ID",http.StatusBadRequest);return}
 var request struct{Revision int64 `json:"revision"`}
 if !decode(w,r,&request){return}
 if request.Revision<1{http.Error(w,"invalid revision",http.StatusBadRequest);return}
 retried,err:=h.Store.RetryFailedSyncForUser(r.Context(),user,subscriptionID,request.Revision)
 if err!=nil{writeError(w,err);return}
 if !retried{http.Error(w,"failed revision not found or no longer retryable",http.StatusConflict);return}
 w.WriteHeader(http.StatusAccepted)
}
