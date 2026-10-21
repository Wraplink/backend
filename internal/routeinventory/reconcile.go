package routeinventory

import (
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "time"

 "github.com/jackc/pgx/v5/pgxpool"
)

// Reconcile atomically plans route bindings from the current enabled policies
// and enabled, recently-seen agent IP inventory. Newly created bindings remain
// pending until an edge agent acknowledges the current generation.
func Reconcile(ctx context.Context, db *pgxpool.Pool, offlineAfter time.Duration) error {
 tx,err:=db.Begin(ctx);if err!=nil{return err}
 defer tx.Rollback(ctx)
 if _,err=tx.Exec(ctx,"SELECT pg_advisory_xact_lock(722311)");err!=nil{return err}
 rows,err:=tx.Query(ctx,"SELECT domain FROM network_domain_policies WHERE mode='route' ORDER BY domain")
 if err!=nil{return err}
 routes:=make([]string,0)
 for rows.Next(){var d string;if err=rows.Scan(&d);err!=nil{rows.Close();return err};routes=append(routes,d)}
 err=rows.Err();rows.Close();if err!=nil{return err}
 rows,err=tx.Query(ctx,`SELECT agent_id,host(public_ip) FROM network_route_agent_ips WHERE enabled=true AND last_seen_at > now() - $1::interval ORDER BY agent_id,public_ip`,fmt.Sprintf("%f seconds",offlineAfter.Seconds()))
 if err!=nil{return err}
 inventory:=map[string][]string{}
 for rows.Next(){var agent,ip string;if err=rows.Scan(&agent,&ip);err!=nil{rows.Close();return err};inventory[agent]=append(inventory[agent],ip)}
 err=rows.Err();rows.Close();if err!=nil{return err}
 rows,err=tx.Query(ctx,`SELECT a.domain,i.agent_id,host(i.public_ip) FROM network_route_assignments a JOIN network_route_agent_ips i ON i.id=a.agent_ip_id ORDER BY a.domain`)
 if err!=nil{return err}
 previous:=make([]Assignment,0)
 for rows.Next(){var a Assignment;if err=rows.Scan(&a.Domain,&a.AgentID,&a.PublicIP);err!=nil{rows.Close();return err};previous=append(previous,a)}
 err=rows.Err();rows.Close();if err!=nil{return err}
 next,_:=Allocate(routes,inventory,previous)
 keep:=map[string]Assignment{}
 for _,a:=range next {keep[a.Domain]=a}
 for _,a:=range previous {
  desired,ok:=keep[a.Domain]
  if ok&&desired.AgentID==a.AgentID&&desired.PublicIP==a.PublicIP {continue}
  if _,err=tx.Exec(ctx,"DELETE FROM network_route_assignments WHERE domain=$1",a.Domain);err!=nil{return err}
 }
 for _,a:=range next {
  found:=false
  for _,old:=range previous {if old==a {found=true;break}}
  if found {continue}
  var raw []byte
  err=tx.QueryRow(ctx,"SELECT route FROM network_domain_policies WHERE domain=$1",a.Domain).Scan(&raw)
  if err!=nil{return err}
  var route struct {
   DestinationIP string `json:"destination_ip"`
   Protocol string `json:"protocol"`
   DestinationPortStart uint16 `json:"destination_port_start"`
   DestinationPortEnd uint16 `json:"destination_port_end"`
  }
  if err=json.Unmarshal(raw,&route);err!=nil{return err}
  if route.DestinationIP=="" {return errors.New("route missing destination IP")}
  _,err=tx.Exec(ctx,`INSERT INTO network_route_assignments(domain,agent_ip_id,destination_ip,protocol,destination_port_start,destination_port_end,state,generation)
  SELECT $1,id,$2::inet,$3,NULLIF($4,0),NULLIF($5,0),'pending',1 FROM network_route_agent_ips WHERE agent_id=$6 AND public_ip=$7::inet`,
   a.Domain,route.DestinationIP,route.Protocol,route.DestinationPortStart,route.DestinationPortEnd,a.AgentID,a.PublicIP)
  if err!=nil{return err}
 }
 return tx.Commit(ctx)
}
