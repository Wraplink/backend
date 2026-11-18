package networksessions

import (
 "crypto/sha256"
 "crypto/subtle"
 "errors"
 "net/http"
 "strings"

)

// AuthenticateAgent binds a request to a provisioned edge node. A shared
// network-control key is deliberately insufficient for this API.
func (r *Repository) AuthenticateAgent(req *http.Request) (string,error) {
 if r==nil || r.DB==nil {return "",errors.New("agent authentication unavailable")}
 code:=strings.TrimSpace(req.Header.Get("X-Network-Agent-ID"))
 auth:=req.Header.Values("Authorization")
 if len(auth)!=1 {return "",errors.New("agent credentials required")}
 parts:=strings.Split(auth[0]," ")
 if len(parts)!=2 || parts[0]!="Bearer" {return "",errors.New("agent credentials required")}
 token:=parts[1]
 if code=="" || len(code)>128 || token=="" || len(token)>512 {
  return "",errors.New("agent credentials required")
 }
 // edge_nodes.code is the stable agent name; network_agent_health.id uses
 // the same string identifier. UUID is resolved server-side, never trusted
 // from a client-provided header.
 var id string
 var expected []byte
 err:=r.DB.QueryRow(req.Context(),`
 SELECT h.id,c.token_sha256 FROM network_agent_health h
 JOIN network_session_agent_credentials c ON c.agent_id=h.id
 WHERE h.id=$1 AND c.enabled
 AND h.status='online' AND h.last_seen_at >= now() - interval '2 minutes'`,code).Scan(&id,&expected)
 if err!=nil {return "",errors.New("invalid agent credentials")}
 actual:=sha256.Sum256([]byte(token))
 if subtle.ConstantTimeCompare(actual[:],expected)!=1 {return "",errors.New("invalid agent credentials")}
 return id,nil
}
