package dnsregistry

import (
 "crypto/subtle"
 "encoding/json"
 "io"
 "net/http"
 "strconv"
 "strings"
 "time"

 "github.com/google/uuid"
)

type DNSQueryEvent struct {
 EventID uuid.UUID `json:"event_id"`
 ObservedAt time.Time `json:"observed_at"`
 Domain string `json:"domain"`
 QType string `json:"qtype"`
 RCode string `json:"rcode"`
 ResolutionMode string `json:"resolution_mode"`
 DurationMS int `json:"duration_ms"`
}
type DNSQueryBatch struct {
 ControllerID string `json:"controller_id"`
 Events []DNSQueryEvent `json:"events"`
}

func validQueryDomain(v string) bool {
 if len(v)<1||len(v)>253||strings.Trim(v,".")=="" {return false}
 for _,label:=range strings.Split(v,".") {
  if len(label)==0||len(label)>63{return false}
  for _,c:=range label {if !(c>='a'&&c<='z'||c>='0'&&c<='9'||c=='-'){return false}}
 }
 return true
}

func (h *Handler) IngestDNSQueries(w http.ResponseWriter,r *http.Request){
 if h.db==nil||!h.Enabled(){writeJSONError(w,503,"query ingestion unavailable");return}
 if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Network-Control-Key")),[]byte(h.controlKey))!=1 {writeJSONError(w,401,"unauthorized");return}
 var batch DNSQueryBatch
 dec:=json.NewDecoder(http.MaxBytesReader(w,r.Body,256<<10));dec.DisallowUnknownFields()
 if dec.Decode(&batch)!=nil {writeJSONError(w,400,"invalid batch");return}
 var extra any;if dec.Decode(&extra)!=io.EOF {writeJSONError(w,400,"unexpected trailing data");return}
 if len(batch.ControllerID)==0||len(batch.ControllerID)>128||len(batch.Events)==0||len(batch.Events)>250 {writeJSONError(w,400,"invalid batch size or controller");return}
 // Only registered controllers may submit events.
 registered:=false
 for _,node:=range h.store.List(0){if node.ID==batch.ControllerID {registered=true;break}}
 if !registered {writeJSONError(w,403,"unregistered DNS controller");return}
 now:=time.Now().UTC()
 for _,e:=range batch.Events {
  if e.EventID==uuid.Nil||!validQueryDomain(e.Domain)||e.Domain!=strings.ToLower(e.Domain)||e.ObservedAt.Before(now.Add(-7*24*time.Hour))||e.ObservedAt.After(now.Add(5*time.Minute))||len(e.QType)>16||len(e.QType)==0||len(e.RCode)>24||len(e.RCode)==0||len(e.ResolutionMode)>32||len(e.ResolutionMode)==0||e.DurationMS<0||e.DurationMS>600000 {writeJSONError(w,400,"invalid query event");return}
 }
 tx,err:=h.db.Begin(r.Context());if err!=nil{writeJSONError(w,500,"database unavailable");return};defer tx.Rollback(r.Context())
 accepted:=0
 for _,e:=range batch.Events {
  tag,err:=tx.Exec(r.Context(),`INSERT INTO dns_query_events(controller_id,event_id,observed_at,domain,qtype,rcode,resolution_mode,duration_ms) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT DO NOTHING`,batch.ControllerID,e.EventID,e.ObservedAt,e.Domain,e.QType,e.RCode,e.ResolutionMode,e.DurationMS)
  if err!=nil{writeJSONError(w,500,"could not store events");return};accepted+=int(tag.RowsAffected())
 }
 if err:=tx.Commit(r.Context());err!=nil{writeJSONError(w,500,"could not commit events");return}
 writeJSON(w,200,map[string]int{"accepted":accepted})
}

func (h *Handler) AdminDNSQueries(w http.ResponseWriter,r *http.Request){
 if h.db==nil{writeJSONError(w,503,"query database unavailable");return}
 q:=r.URL.Query()
 page,_:=strconv.Atoi(q.Get("page"));if page<1{page=1}
 size,_:=strconv.Atoi(q.Get("pageSize"));if size<1{size=25};if size>100{size=100}
 if page>100000 {writeJSONError(w,400,"page too large");return}
 search:=strings.TrimSpace(q.Get("search"));controller:=strings.TrimSpace(q.Get("controller"));qtype:=strings.TrimSpace(q.Get("qtype"));rcode:=strings.TrimSpace(q.Get("rcode"));mode:=strings.TrimSpace(q.Get("mode"));managed:=strings.TrimSpace(q.Get("managed"))
 if managed!=""&&managed!="true"&&managed!="false" {writeJSONError(w,400,"invalid managed filter");return}
 if len(search)>253||len(controller)>128||len(qtype)>16||len(rcode)>24||len(mode)>32{writeJSONError(w,400,"invalid filters");return}
 var total int64
 where:=` FROM dns_query_events e LEFT JOIN LATERAL (SELECT domain, mode FROM network_domain_policies WHERE domain=e.domain OR e.domain LIKE '%.' || domain ORDER BY length(domain) DESC LIMIT 1) p ON true WHERE ($1='' OR e.domain ILIKE '%' || $1 || '%') AND ($2='' OR e.controller_id=$2) AND ($3='' OR e.qtype=$3) AND ($4='' OR e.rcode=$4) AND ($5='' OR e.resolution_mode=$5) AND ($6='' OR ($6='true' AND p.domain IS NOT NULL) OR ($6='false' AND p.domain IS NULL))`
 args:=[]any{search,controller,qtype,rcode,mode,managed}
 if err:=h.db.QueryRow(r.Context(),"SELECT count(*)"+where,args...).Scan(&total);err!=nil{writeJSONError(w,500,"could not count queries");return}
 rows,err:=h.db.Query(r.Context(),"SELECT e.controller_id,e.event_id,e.observed_at,e.domain,e.qtype,e.rcode,e.resolution_mode,e.duration_ms,(p.domain IS NOT NULL),COALESCE(p.mode,''),COALESCE(p.domain,'')"+where+" ORDER BY e.observed_at DESC,e.event_id DESC LIMIT $7 OFFSET $8",append(args,size,(page-1)*size)...)
 if err!=nil{writeJSONError(w,500,"could not list queries");return};defer rows.Close()
 type item struct {ControllerID string `json:"controllerId"`;EventID uuid.UUID `json:"eventId"`;ObservedAt time.Time `json:"observedAt"`;Domain string `json:"domain"`;QType string `json:"qtype"`;RCode string `json:"rcode"`;ResolutionMode string `json:"resolutionMode"`;DurationMS int `json:"durationMs"`;Managed bool `json:"managed"`;ManagedMode string `json:"managedMode"`;ManagedDomain string `json:"managedDomain"`}
 result:=make([]item,0)
 for rows.Next(){var e item;if err:=rows.Scan(&e.ControllerID,&e.EventID,&e.ObservedAt,&e.Domain,&e.QType,&e.RCode,&e.ResolutionMode,&e.DurationMS,&e.Managed,&e.ManagedMode,&e.ManagedDomain);err!=nil{writeJSONError(w,500,"could not read queries");return};result=append(result,e)}
 if rows.Err()!=nil{writeJSONError(w,500,"could not read queries");return}
 writeJSON(w,200,map[string]any{"queries":result,"total":total,"page":page,"pageSize":size})
}
