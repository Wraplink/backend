package dnsdomains

import (
 "encoding/json"
 "net"
 "net/http"
 "strings"
)

type Mode string
const (
 SNIHijack Mode = "SNI_HIJACK"
 FullRoute Mode = "FULL_ROUTE"
 Direct Mode = "DIRECT"
)
type DomainInput struct {
 Domain string `json:"domain"`
 Enabled bool `json:"enabled"`
 Mode Mode `json:"mode"`
 AgentID *string `json:"agentId"`
 OverrideIP *string `json:"overrideIp"`
 FallbackMode string `json:"fallbackMode"`
 Priority int `json:"priority"`
}
func (d DomainInput) Validate() map[string]string {
 errors := map[string]string{}
 domain := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(d.Domain)), ".")
 if len(domain)==0 || len(domain)>253 || net.ParseIP(domain)!=nil { errors["domain"]="invalid domain" } else {
  for _,label := range strings.Split(domain,".") {
   if len(label)==0 || len(label)>63 || label[0]=='-' || label[len(label)-1]=='-' {errors["domain"]="invalid DNS label";break}
   for _,r := range label {if !(r>='a'&&r<='z'||r>='0'&&r<='9'||r=='-'){errors["domain"]="invalid DNS label";break}}
  }
 }
 switch d.Mode {case SNIHijack,FullRoute,Direct: default: errors["mode"]="unsupported resolution mode"}
 if d.OverrideIP!=nil && net.ParseIP(*d.OverrideIP)==nil {errors["overrideIp"]="invalid IP address"}
 if d.FallbackMode!="" && d.FallbackMode!="DIRECT" && d.FallbackMode!="REFUSE" {errors["fallbackMode"]="unsupported fallback"}
 if d.Priority<0 {errors["priority"]="must not be negative"}
 return errors
}
func DecodeInput(r *http.Request) (DomainInput,map[string]string,error) {
 var input DomainInput
 decoder:=json.NewDecoder(r.Body)
 decoder.DisallowUnknownFields()
 err:=decoder.Decode(&input)
 if err!=nil{return input,nil,err}
 input.Domain=strings.TrimSuffix(strings.ToLower(strings.TrimSpace(input.Domain)),".")
 if input.FallbackMode=="" {input.FallbackMode="DIRECT"}
 return input,input.Validate(),nil
}
