package starterpolicy

import (
 "encoding/json"
 "errors"
 "io"
 "net/http"
 "time"

 "github.com/go-chi/chi/v5"
 "github.com/google/uuid"
)

type AdminHandler struct { Store PolicyStore }
func (h AdminHandler) List(w http.ResponseWriter,r *http.Request) {
 items,err:=h.Store.List(r.Context())
 if err!=nil {adminError(w,500,"could not list starter policies");return}
 type item struct {
  AdminPolicy
  ReservationTTLSeconds int64 `json:"reservationTtlSeconds"`
  TrialDurationSeconds int64 `json:"trialDurationSeconds"`
 }
 result:=make([]item,0,len(items))
 for _,p:=range items {result=append(result,item{p,int64(p.ReservationTTL/time.Second),int64(p.TrialDuration/time.Second)})}
 adminJSON(w,200,map[string]any{"policies":result})
}
func (h AdminHandler) Create(w http.ResponseWriter,r *http.Request) {
 var body struct {
  PlanID uuid.UUID `json:"planId"`
  Enabled bool `json:"enabled"`
  Priority int `json:"priority"`
  Region string `json:"region"`
  AssignmentTrigger Trigger `json:"assignmentTrigger"`
  ActivationTrigger string `json:"activationTrigger"`
  ReservationTTLSeconds int64 `json:"reservationTtlSeconds"`
  TrialDurationSeconds int64 `json:"trialDurationSeconds"`
 }
 dec:=json.NewDecoder(http.MaxBytesReader(w,r.Body,16<<10))
 dec.DisallowUnknownFields()
 if err:=dec.Decode(&body);err!=nil {adminError(w,400,"invalid request");return}
 var extra any
 if err:=dec.Decode(&extra);!errors.Is(err,io.EOF){adminError(w,400,"invalid request");return}
 if body.ReservationTTLSeconds<=0 || body.ReservationTTLSeconds>365*24*3600 ||
  body.TrialDurationSeconds<=0 || body.TrialDurationSeconds>365*24*3600 {
  adminError(w,400,"invalid duration");return
 }
 p,err:=h.Store.Create(r.Context(),AdminPolicy{
  PlanID:body.PlanID,Enabled:body.Enabled,Priority:body.Priority,Region:body.Region,
  AssignmentTrigger:body.AssignmentTrigger,ActivationTrigger:body.ActivationTrigger,
  ReservationTTL:time.Duration(body.ReservationTTLSeconds)*time.Second,
  TrialDuration:time.Duration(body.TrialDurationSeconds)*time.Second,
 })
 if err!=nil {
  if errors.Is(err,ErrInvalidPolicy){adminError(w,400,"invalid starter policy");return}
  if errors.Is(err,ErrNotEligible){adminError(w,422,"plan must be active and free");return}
  adminError(w,500,"could not create starter policy");return
 }
 adminJSON(w,201,map[string]any{"id":p.ID})
}
func (h AdminHandler) SetEnabled(w http.ResponseWriter,r *http.Request) {
 id,err:=uuid.Parse(chi.URLParam(r,"policyID"))
 if err!=nil || id==uuid.Nil {adminError(w,400,"invalid policy ID");return}
 var body struct {Enabled *bool `json:"enabled"`}
 dec:=json.NewDecoder(http.MaxBytesReader(w,r.Body,4096))
 dec.DisallowUnknownFields()
 if err:=dec.Decode(&body);err!=nil || body.Enabled==nil {adminError(w,400,"invalid request");return}
 var extra any
 if err:=dec.Decode(&extra);!errors.Is(err,io.EOF){adminError(w,400,"invalid request");return}
 err=h.Store.SetEnabled(r.Context(),id,*body.Enabled)
 if errors.Is(err,ErrNotEligible){adminError(w,404,"policy not found");return}
 if err!=nil{adminError(w,500,"could not update starter policy");return}
 adminJSON(w,200,map[string]any{"updated":true})
}
func adminJSON(w http.ResponseWriter,status int,data any){
 w.Header().Set("Content-Type","application/json")
 w.WriteHeader(status)
 _=json.NewEncoder(w).Encode(data)
}
func adminError(w http.ResponseWriter,status int,message string){
 adminJSON(w,status,map[string]string{"error":message})
}
