package slackreport

import (
 "crypto/subtle"
 "encoding/json"
 "net/http"
 "os"
 "strings"
 "time"

 "github.com/jackc/pgx/v5/pgxpool"
)

type AlertHandler struct { DB *pgxpool.Pool; Key string }
func NewAlertHandler(db *pgxpool.Pool) *AlertHandler {
 return &AlertHandler{DB:db, Key:strings.TrimSpace(os.Getenv("WRAPLINK_ALERT_INGEST_KEY"))}
}
func (h *AlertHandler) Enabled() bool {return h.Key!=""}
func (h *AlertHandler) Ingest(w http.ResponseWriter,r *http.Request) {
 if !h.Enabled(){http.NotFound(w,r);return}
 provided:=r.Header.Get("X-Wraplink-Alert-Key")
 if len(provided)!=len(h.Key)||subtle.ConstantTimeCompare([]byte(provided),[]byte(h.Key))!=1 {
  http.Error(w,"unauthorized",http.StatusUnauthorized);return
 }
 var a struct {
  Service string `json:"service"`
  Severity string `json:"severity"`
  Code string `json:"code"`
  Fingerprint string `json:"fingerprint"`
  Summary string `json:"summary"`
  Environment string `json:"environment"`
 }
 dec:=json.NewDecoder(http.MaxBytesReader(w,r.Body,4096));dec.DisallowUnknownFields()
 if err:=dec.Decode(&a);err!=nil{http.Error(w,"invalid alert",400);return}
 validService:=map[string]bool{"backend":true,"dns-control":true,"edge-agent":true,"web":true}
 validSeverity:=map[string]bool{"warning":true,"error":true,"critical":true,"recovery":true}
 if !validService[a.Service]||!validSeverity[a.Severity]||len(a.Code)<1||len(a.Code)>80||len(a.Fingerprint)<1||len(a.Fingerprint)>120||len(a.Summary)<1||len(a.Summary)>240||len(a.Environment)>32 {
  http.Error(w,"invalid alert fields",400);return
 }
 // Summary is a predefined safe operational label, not a raw exception or stack trace.
 for _,v:=range []string{a.Code,a.Fingerprint,a.Summary,a.Environment}{
  if strings.ContainsAny(v,"\n\r<>") {http.Error(w,"invalid alert characters",400);return}
 }
 bucket:=time.Now().UTC().Unix()/300
 key:=a.Service+":"+a.Severity+":"+a.Fingerprint+":"+strings.TrimSpace(a.Environment)+":"+time.Unix(bucket*300,0).UTC().Format(time.RFC3339)
 payload,_:=json.Marshal(map[string]string{"text":"WrapLink "+a.Severity+" · "+a.Service+" ["+a.Environment+"] · "+a.Code+": "+a.Summary})
 _,err:=h.DB.Exec(r.Context(),`INSERT INTO slack_outbox(event_key,event_type,payload) VALUES($1,'runtime.alert',$2) ON CONFLICT(event_key) DO NOTHING`,key,string(payload))
 if err!=nil{http.Error(w,"alert queue unavailable",503);return}
 w.Header().Set("Content-Type","application/json")
 w.WriteHeader(http.StatusAccepted)
 _,_=w.Write([]byte(`{"queued":true}`))
}
