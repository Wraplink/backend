package dnsdomains

import (
 "crypto/subtle"
 "encoding/json"
 "errors"
 "io"
 "net/http"
 "github.com/go-chi/chi/v5"
 "github.com/google/uuid"
 "github.com/jackc/pgx/v5/pgconn"
)
type Handler struct { Store Store; ControlKey string }
func respond(w http.ResponseWriter,status int,value any){w.Header().Set("Content-Type","application/json");w.WriteHeader(status);_ = json.NewEncoder(w).Encode(value)}
func (h Handler) List(w http.ResponseWriter,r *http.Request){result,err:=h.Store.List(r.Context());if err!=nil{respond(w,500,map[string]string{"error":"internal error"});return};respond(w,200,result)}
func (h Handler) Create(w http.ResponseWriter,r *http.Request){
 input,ok:=parse(w,r);if !ok{return}
 result,err:=h.Store.Create(r.Context(),input);if err!=nil{databaseError(w,err);return};respond(w,201,result)
}
func (h Handler) Update(w http.ResponseWriter,r *http.Request){
 id:=chi.URLParam(r,"id");if _,err:=uuid.Parse(id);err!=nil{respond(w,400,map[string]string{"error":"invalid id"});return}
 input,ok:=parse(w,r);if !ok{return}
 result,err:=h.Store.Update(r.Context(),id,input);if err!=nil{databaseError(w,err);return};respond(w,200,result)
}
func (h Handler) Delete(w http.ResponseWriter,r *http.Request){
 id:=chi.URLParam(r,"id");if _,err:=uuid.Parse(id);err!=nil{respond(w,400,map[string]string{"error":"invalid id"});return}
 if err:=h.Store.Delete(r.Context(),id);err!=nil{databaseError(w,err);return};w.WriteHeader(204)
}
func parse(w http.ResponseWriter,r *http.Request)(DomainInput,bool){
 r.Body=http.MaxBytesReader(w,r.Body,1<<20)
 input,issues,err:=DecodeInput(r)
 if err!=nil{respond(w,400,map[string]string{"error":"invalid JSON"});return input,false}
 var extra any
 if err:=json.NewDecoder(io.LimitReader(r.Body,1)).Decode(&extra);err!=io.EOF{respond(w,400,map[string]string{"error":"unexpected trailing data"});return input,false}
 if len(issues)>0{respond(w,400,map[string]any{"errors":issues});return input,false}
 return input,true
}
func databaseError(w http.ResponseWriter,err error){
 if errors.Is(err,ErrNotFound){respond(w,404,map[string]string{"error":"not found"});return}
 var pgerr *pgconn.PgError
 if errors.As(err,&pgerr) && (pgerr.Code=="23505" || pgerr.Code=="23503" || pgerr.Code=="23514" || pgerr.Code=="22P02"){respond(w,409,map[string]string{"error":"domain rule conflict or invalid reference"});return}
 respond(w,500,map[string]string{"error":"internal error"})
}
// Mount only within the existing authenticated, admin-authorized router group.
func (h Handler) Routes(r chi.Router){
 r.Get("/",h.List)
 r.Post("/",h.Create)
 r.Put("/{id}",h.Update)
 r.Delete("/{id}",h.Delete)
}

 // Snapshot exposes managed policies to authenticated control-plane nodes.
 // It is a separate v2 contract: legacy DNS policy consumers remain unchanged.
func (h Handler) Snapshot(w http.ResponseWriter,r *http.Request){
 if h.ControlKey=="" || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Network-Control-Key")),[]byte(h.ControlKey))!=1 {
  respond(w,http.StatusUnauthorized,map[string]string{"error":"invalid network control key"});return
 }
 items,err:=h.Store.List(r.Context())
 if err!=nil {respond(w,http.StatusInternalServerError,map[string]string{"error":"cannot load managed domains"});return}
 active:=make([]Domain,0,len(items))
 for _,item:=range items {if item.Enabled {active=append(active,item)}}
 respond(w,http.StatusOK,map[string]any{"schemaVersion":2,"domains":active})
}
