package routeinventory

import (
 "net"
 "sort"
 "strings"
)

// Assignment represents the desired exclusive public-IP binding for one route.
type Assignment struct {
 Domain string `json:"domain"`
 AgentID string `json:"agentId"`
 PublicIP string `json:"publicIp"`
}

// Allocate assigns waiting routes to available addresses deterministically.
// Existing assignments are retained only when their agent and IP are still
// advertised. The caller must persist the result atomically with generation
// fencing before publishing it to DNS Control.
func Allocate(routes []string, inventory map[string][]string, existing []Assignment) ([]Assignment, []string) {
 valid := make(map[string]struct{},len(routes))
 for _,d:=range routes {d=strings.ToLower(strings.TrimSpace(d));if d!="" {valid[d]=struct{}{}}}
 agents:=make([]string,0,len(inventory))
 for id:=range inventory {agents=append(agents,id)}
 sort.Strings(agents)
 owner:=make(map[string]string)
 for _,id:=range agents {
  ips:=append([]string(nil),inventory[id]...)
  sort.Strings(ips)
  for _,address:=range ips {
   ip:=net.ParseIP(address)
   if ip==nil||ip.To4()==nil {continue}
   key:=ip.To4().String()
   if _,exists:=owner[key];!exists {owner[key]=id}
  }
 }
 used:=make(map[string]bool)
 assigned:=make(map[string]Assignment)
 for _,a:=range existing {
  ip:=net.ParseIP(a.PublicIP)
  if ip==nil||ip.To4()==nil {continue}
  address:=ip.To4().String()
  if _,ok:=valid[a.Domain];!ok||used[address]||assigned[a.Domain].Domain!=""||owner[address]!=a.AgentID {continue}
  a.PublicIP=address
  assigned[a.Domain]=a
  used[address]=true
 }
 free:=make([]string,0,len(owner))
 for ip:=range owner {if !used[ip] {free=append(free,ip)}}
 sort.Slice(free,func(i,j int)bool {
  if owner[free[i]]==owner[free[j]] {return free[i]<free[j]}
  return owner[free[i]]<owner[free[j]]
 })
 names:=make([]string,0,len(valid))
 for d:=range valid {names=append(names,d)}
 sort.Strings(names)
 waiting:=make([]string,0)
 for _,d:=range names {
  if _,ok:=assigned[d];ok {continue}
  if len(free)==0 {waiting=append(waiting,d);continue}
  ip:=free[0];free=free[1:]
  assigned[d]=Assignment{Domain:d,AgentID:owner[ip],PublicIP:ip}
 }
 result:=make([]Assignment,0,len(assigned))
 for _,d:=range names {if a,ok:=assigned[d];ok {result=append(result,a)}}
 return result,waiting
}
