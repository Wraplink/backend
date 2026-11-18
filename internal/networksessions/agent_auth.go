package networksessions

import (
 "crypto/sha256"
 "crypto/subtle"
 "errors"
 "net/http"
 "strings"

 "github.com/google/uuid"
)

// AuthenticateAgent binds a request to a provisioned edge node. A shared
// network-control key is deliberately insufficient for this API.
func (r *Repository) AuthenticateAgent(req *http.Request) (uuid.UUID,error) {
 if r==nil || r.DB==nil {return uuid.Nil,errors.New("agent authentication unavailable")}
 code:=strings.TrimSpace(req.Header.Get("X-Network-Agent-ID"))
 token:=strings.TrimSpace(strings.TrimPrefix(req.Header.Get("Authorization"),"Bearer "))
 if code=="" || len(code)>128 || token=="" || len(token)>512 || !strings.HasPrefix(req.Header.Get("Authorization"),"Bearer ") {
  return uuid.Nil,errors.New("agent credentials required")
 }
 // edge_nodes.code is the stable agent name; network_agent_health.id uses
 // the same string identifier. UUID is resolved server-side, never trusted
 // from a client-provided header.
 var id uuid.UUID
 var expected []byte
 err:=r.DB.QueryRow(req.Context(),`
 SELECT e.id,c.token_sha256 FROM edge_nodes e
 JOIN network_session_agent_credentials c ON c.edge_node_id=e.id
 WHERE e.code=$1 AND e.enabled AND c.enabled`,code).Scan(&id,&expected)
 if err!=nil {return uuid.Nil,errors.New("invalid agent credentials")}
 actual:=sha256.Sum256([]byte(token))
 if subtle.ConstantTimeCompare(actual[:],expected)!=1 {return uuid.Nil,errors.New("invalid agent credentials")}
 return id,nil
}
