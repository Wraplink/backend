package dnsregistry

import (
 "crypto/subtle"
 "encoding/json"
 "io"
 "log/slog"
 "net"
 "net/http"
 "strconv"
 "backend/internal/routeinventory"
 "time"
 "strings"

 "github.com/go-chi/chi/v5"
)

type DomainPolicy struct {
 Domain string `json:"domain"`
 Mode string `json:"mode"`
 SpecificIP string `json:"specific_ip,omitempty"`
 Route *RoutePolicy `json:"route,omitempty"`
 ConnectionModes []string `json:"connection_modes,omitempty"`
 Assignment *RouteAssignment `json:"assignment,omitempty"`
}
type RouteAssignment struct {
 AgentID string `json:"agent_id"`
 PublicIP string `json:"public_ip"`
 State string `json:"state"`
 Generation int64 `json:"generation"`
}
type RoutePolicy struct {
 DestinationIP string `json:"destination_ip"`
 Protocol string `json:"protocol"`
 DestinationPortStart uint16 `json:"destination_port_start,omitempty"`
 DestinationPortEnd uint16 `json:"destination_port_end,omitempty"`
}
func validPolicy(p DomainPolicy) bool {
 if len(p.Domain)==0 || len(p.Domain)>253 || strings.ContainsAny(p.Domain," /:@") || strings.HasPrefix(p.Domain,".") || strings.HasSuffix(p.Domain,".") {return false}
 for _,label:=range strings.Split(p.Domain,".") {if len(label)==0||len(label)>63 {return false};for _,r:=range label {if !((r>='a'&&r<='z')||(r>='0'&&r<='9')||r=='-') {return false}};if label[0]=='-'||label[len(label)-1]=='-' {return false}}
 for _,m:=range p.ConnectionModes {if m!="smart"&&m!="gaming"&&m!="streaming" {return false}}
 switch p.Mode {
 case "normal","hijack","block":return p.SpecificIP==""&&p.Route==nil
 case "specific_ip":return net.ParseIP(p.SpecificIP)!=nil&&p.Route==nil
 case "route":
  if p.Route==nil||p.SpecificIP!="" {return false}
  if p.Route.DestinationIP!="" && (net.ParseIP(p.Route.DestinationIP)==nil||net.ParseIP(p.Route.DestinationIP).To4()==nil) {return false}
  if p.Route.Protocol!="tcp"&&p.Route.Protocol!="udp"&&p.Route.Protocol!="tcp_udp" {return false}
  return (p.Route.DestinationPortStart==0&&p.Route.DestinationPortEnd==0)||(p.Route.DestinationPortStart>0&&p.Route.DestinationPortEnd>=p.Route.DestinationPortStart)
 }
 return false
}
func (h *Handler) listPolicies(w http.ResponseWriter,r *http.Request,paginate bool) {
 if h.db==nil {writeJSONError(w,503,"domain policy database unavailable");return}
 query:=strings.TrimSpace(r.URL.Query().Get("search"))
 modeFilter:=strings.TrimSpace(r.URL.Query().Get("mode"))
 if !paginate {query="";modeFilter=""}
 if modeFilter!="" {
  switch modeFilter {
  case "normal","hijack","route","specific_ip","block":
  default:
   writeJSONError(w,http.StatusBadRequest,"invalid resolution mode filter")
   return
  }
 }
 page,err:=strconv.Atoi(r.URL.Query().Get("page"))
 if err!=nil || page<1 {page=1}
 pageSize,err:=strconv.Atoi(r.URL.Query().Get("pageSize"))
 if err!=nil || pageSize<1 {pageSize=25}
 if pageSize>100 {pageSize=100}
 if !paginate {pageSize=2147483647;page=1}
 offset:=(int64(page)-1)*int64(pageSize)
 if offset<0 {writeJSONError(w,400,"invalid page");return}
 var total int64
 if err:=h.db.QueryRow(r.Context(),`SELECT count(*) FROM network_domain_policies WHERE ($1='' OR domain ILIKE '%' || $1 || '%') AND ($2='' OR mode=$2)`,query,modeFilter).Scan(&total);err!=nil {writeJSONError(w,500,"cannot count domain policies");return}
 rows,err:=h.db.Query(r.Context(),`SELECT p.domain,p.mode,COALESCE(host(p.specific_ip),''),p.route,p.connection_modes,
 COALESCE(i.agent_id,''),COALESCE(host(i.public_ip),''),COALESCE(a.state,''),COALESCE(a.generation,0),COALESCE(host(a.destination_ip),''),COALESCE(a.protocol,''),COALESCE(a.destination_port_start,0),COALESCE(a.destination_port_end,0)
 FROM network_domain_policies p
 LEFT JOIN network_route_assignments a ON a.domain=p.domain
 LEFT JOIN network_route_agent_ips i ON i.id=a.agent_ip_id
 WHERE ($1='' OR p.domain ILIKE '%' || $1 || '%') AND ($4='' OR p.mode=$4) ORDER BY p.domain LIMIT $2 OFFSET $3`,query,pageSize,offset,modeFilter)
 if err!=nil {writeJSONError(w,500,"cannot list domain policies");return}
 defer rows.Close()
 policies:=make([]DomainPolicy,0)
 for rows.Next(){
  var p DomainPolicy;var raw []byte
  var assignment RouteAssignment
  var assignedDestinationIP,assignedProtocol string
  var assignedPortStart,assignedPortEnd uint16
  if err:=rows.Scan(&p.Domain,&p.Mode,&p.SpecificIP,&raw,&p.ConnectionModes,&assignment.AgentID,&assignment.PublicIP,&assignment.State,&assignment.Generation,&assignedDestinationIP,&assignedProtocol,&assignedPortStart,&assignedPortEnd);err!=nil {writeJSONError(w,500,"cannot read domain policy");return}
  if len(raw)>0 {var route RoutePolicy;if err:=json.Unmarshal(raw,&route);err!=nil {writeJSONError(w,500,"invalid stored route");return};p.Route=&route}
  if p.Mode=="route" && assignment.AgentID!="" && assignment.PublicIP!="" && assignment.Generation>0 && p.Route!=nil {
   // The route policy may omit destination_ip; the reconciler resolves it independently.
   // Publish the exact destination and protocol actually assigned to the edge.
   if !paginate {
    p.Route.DestinationIP=assignedDestinationIP
    p.Route.Protocol=assignedProtocol
    p.Route.DestinationPortStart=assignedPortStart
    p.Route.DestinationPortEnd=assignedPortEnd
   }
   p.Assignment=&assignment
  }
  policies=append(policies,p)
 }
 if rows.Err()!=nil {writeJSONError(w,500,"cannot read domain policies");return}
 var revision int64
 if err:=h.db.QueryRow(r.Context(),"SELECT revision FROM network_domain_policy_revision WHERE singleton=TRUE").Scan(&revision);err!=nil {writeJSONError(w,500,"cannot read policy revision");return}
 writeJSON(w,200,map[string]any{"revision":revision,"domains":policies,"total":total,"page":page,"pageSize":pageSize})
}
func (h *Handler) AdminPolicies(w http.ResponseWriter,r *http.Request){h.listPolicies(w,r,true)}
func (h *Handler) SnapshotPolicies(w http.ResponseWriter,r *http.Request){
 if h.controlKey==""||subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Network-Control-Key")),[]byte(h.controlKey))!=1 {writeJSONError(w,401,"invalid network control key");return}
 h.listPolicies(w,r,false)
}
func (h *Handler) PutPolicy(w http.ResponseWriter,r *http.Request){
 if h.db==nil {writeJSONError(w,503,"domain policy database unavailable");return}
 var p DomainPolicy
 dec:=json.NewDecoder(http.MaxBytesReader(w,r.Body,16384));dec.DisallowUnknownFields()
 if err:=dec.Decode(&p);err!=nil {writeJSONError(w,400,"invalid domain policy");return}
 var extra any;if dec.Decode(&extra)!=io.EOF {writeJSONError(w,400,"unexpected request content");return}
 p.Domain=strings.ToLower(strings.TrimSpace(p.Domain));p.SpecificIP=strings.TrimSpace(p.SpecificIP)
 if !validPolicy(p) {writeJSONError(w,400,"invalid domain, mode, address, route or connection modes");return}
 if p.ConnectionModes==nil {p.ConnectionModes=[]string{}}
 var route any
 if p.Route!=nil {encoded,err:=json.Marshal(p.Route);if err!=nil {writeJSONError(w,400,"invalid route");return};route=string(encoded)}
 _,err:=h.db.Exec(r.Context(),`INSERT INTO network_domain_policies(domain,mode,specific_ip,route,connection_modes) VALUES($1,$2,NULLIF($3,'')::inet,$4::jsonb,$5::text[])
 ON CONFLICT(domain) DO UPDATE SET mode=excluded.mode,specific_ip=excluded.specific_ip,route=excluded.route,connection_modes=excluded.connection_modes,updated_at=now()`,p.Domain,p.Mode,p.SpecificIP,route,p.ConnectionModes)
 if err==nil {if reconcileErr:=routeinventory.Reconcile(r.Context(),h.db,90*time.Second);reconcileErr!=nil {slog.Warn("domain policy saved; route allocation pending", "domain",p.Domain,"error",reconcileErr)}}
 if err!=nil {slog.Error("failed to save domain policy","domain",p.Domain,"mode",p.Mode,"error",err);writeJSONError(w,500,"cannot save domain policy");return}
 writeJSON(w,200,p)
}
func (h *Handler) DeletePolicy(w http.ResponseWriter,r *http.Request){
 if h.db==nil {writeJSONError(w,503,"domain policy database unavailable");return}
 domain:=strings.ToLower(strings.TrimSpace(chi.URLParam(r,"domain")))
 if !validPolicy(DomainPolicy{Domain:domain,Mode:"normal"}) {writeJSONError(w,400,"invalid domain");return}
 tag,err:=h.db.Exec(r.Context(),"DELETE FROM network_domain_policies WHERE domain=$1",domain)
 if err!=nil {writeJSONError(w,500,"cannot delete domain policy");return}
 if tag.RowsAffected()==0 {writeJSONError(w,404,"domain policy not found");return}
 if err=routeinventory.Reconcile(r.Context(),h.db,90*time.Second);err!=nil {writeJSONError(w,503,"domain removed but route reconciliation failed");return}
 w.WriteHeader(http.StatusNoContent)
}
