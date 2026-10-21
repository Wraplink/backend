package dnsregistry

import (
 "crypto/subtle"
 "encoding/json"
 "io"
 "net"
 "net/http"
 "strings"

 "github.com/go-chi/chi/v5"
)

type DomainPolicy struct {
 Domain string `json:"domain"`
 Mode string `json:"mode"`
 SpecificIP string `json:"specific_ip,omitempty"`
 Route *RoutePolicy `json:"route,omitempty"`
 ConnectionModes []string `json:"connection_modes,omitempty"`
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
 case "normal","hijack":return p.SpecificIP==""&&p.Route==nil
 case "specific_ip":return net.ParseIP(p.SpecificIP)!=nil&&p.Route==nil
 case "route":
  if p.Route==nil||p.SpecificIP!=""||net.ParseIP(p.Route.DestinationIP)==nil||net.ParseIP(p.Route.DestinationIP).To4()==nil {return false}
  if p.Route.Protocol!="tcp"&&p.Route.Protocol!="udp" {return false}
  return (p.Route.DestinationPortStart==0&&p.Route.DestinationPortEnd==0)||(p.Route.DestinationPortStart>0&&p.Route.DestinationPortEnd>=p.Route.DestinationPortStart)
 }
 return false
}
func (h *Handler) listPolicies(w http.ResponseWriter,r *http.Request) {
 if h.db==nil {writeJSONError(w,503,"domain policy database unavailable");return}
 rows,err:=h.db.Query(r.Context(),`SELECT domain,mode,COALESCE(host(specific_ip),''),route,connection_modes FROM network_domain_policies ORDER BY domain`)
 if err!=nil {writeJSONError(w,500,"cannot list domain policies");return}
 defer rows.Close()
 policies:=make([]DomainPolicy,0)
 for rows.Next(){
  var p DomainPolicy;var raw []byte
  if err:=rows.Scan(&p.Domain,&p.Mode,&p.SpecificIP,&raw,&p.ConnectionModes);err!=nil {writeJSONError(w,500,"cannot read domain policy");return}
  if len(raw)>0 {var route RoutePolicy;if err:=json.Unmarshal(raw,&route);err!=nil {writeJSONError(w,500,"invalid stored route");return};p.Route=&route}
  policies=append(policies,p)
 }
 if rows.Err()!=nil {writeJSONError(w,500,"cannot read domain policies");return}
 var revision int64
 if err:=h.db.QueryRow(r.Context(),"SELECT revision FROM network_domain_policy_revision WHERE singleton=TRUE").Scan(&revision);err!=nil {writeJSONError(w,500,"cannot read policy revision");return}
 writeJSON(w,200,map[string]any{"revision":revision,"domains":policies})
}
func (h *Handler) AdminPolicies(w http.ResponseWriter,r *http.Request){h.listPolicies(w,r)}
func (h *Handler) SnapshotPolicies(w http.ResponseWriter,r *http.Request){
 if h.controlKey==""||subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Network-Control-Key")),[]byte(h.controlKey))!=1 {writeJSONError(w,401,"invalid network control key");return}
 h.listPolicies(w,r)
}
func (h *Handler) PutPolicy(w http.ResponseWriter,r *http.Request){
 if h.db==nil {writeJSONError(w,503,"domain policy database unavailable");return}
 var p DomainPolicy
 dec:=json.NewDecoder(http.MaxBytesReader(w,r.Body,16384));dec.DisallowUnknownFields()
 if err:=dec.Decode(&p);err!=nil {writeJSONError(w,400,"invalid domain policy");return}
 var extra any;if dec.Decode(&extra)!=io.EOF {writeJSONError(w,400,"unexpected request content");return}
 p.Domain=strings.ToLower(strings.TrimSpace(p.Domain));p.SpecificIP=strings.TrimSpace(p.SpecificIP)
 if !validPolicy(p) {writeJSONError(w,400,"invalid domain, mode, address, route or connection modes");return}
 var route any
 if p.Route!=nil {encoded,err:=json.Marshal(p.Route);if err!=nil {writeJSONError(w,400,"invalid route");return};route=encoded}
 _,err:=h.db.Exec(r.Context(),`INSERT INTO network_domain_policies(domain,mode,specific_ip,route,connection_modes) VALUES($1,$2,NULLIF($3,'')::inet,$4,$5)
 ON CONFLICT(domain) DO UPDATE SET mode=excluded.mode,specific_ip=excluded.specific_ip,route=excluded.route,connection_modes=excluded.connection_modes,updated_at=now()`,p.Domain,p.Mode,p.SpecificIP,route,p.ConnectionModes)
 if err!=nil {writeJSONError(w,500,"cannot save domain policy");return}
 writeJSON(w,200,p)
}
func (h *Handler) DeletePolicy(w http.ResponseWriter,r *http.Request){
 if h.db==nil {writeJSONError(w,503,"domain policy database unavailable");return}
 domain:=strings.ToLower(strings.TrimSpace(chi.URLParam(r,"domain")))
 if !validPolicy(DomainPolicy{Domain:domain,Mode:"normal"}) {writeJSONError(w,400,"invalid domain");return}
 tag,err:=h.db.Exec(r.Context(),"DELETE FROM network_domain_policies WHERE domain=$1",domain)
 if err!=nil {writeJSONError(w,500,"cannot delete domain policy");return}
 if tag.RowsAffected()==0 {writeJSONError(w,404,"domain policy not found");return}
 w.WriteHeader(http.StatusNoContent)
}
