package publicip

import (
 "errors"
 "fmt"

 "github.com/google/uuid"
)

// NodeAcknowledgement represents a node's reported effective enforcement
// state. It must come from authenticated control-plane state, not a client.
type NodeAcknowledgement struct {
 NodeID string
 SubscriptionID uuid.UUID
 PublicIPID uuid.UUID
 Revision int64
 Applied bool
}

// ErrIncompleteNetworkAcknowledgement means the desired revision is not
// proven active across all required nodes.
var ErrIncompleteNetworkAcknowledgement = errors.New("required network acknowledgement missing or inconsistent")

// ValidateNetworkAcknowledgements checks an already authenticated set of node
// reports. It does not fetch, authenticate, or persist reports; those are
// mandatory responsibilities of the future control-plane integration.
func ValidateNetworkAcknowledgements(job SyncJob, required []string, reports []NodeAcknowledgement) error {
 if job.SubscriptionID==uuid.Nil || job.PublicIPID==uuid.Nil || job.Revision<1 || len(required)==0 {
  return ErrIncompleteNetworkAcknowledgement
 }
 expected:=make(map[string]struct{},len(required))
 for _,node:=range required {
  if node=="" {return ErrIncompleteNetworkAcknowledgement}
  if _,duplicate:=expected[node];duplicate{return ErrIncompleteNetworkAcknowledgement}
  expected[node]=struct{}{}
 }
 seen:=make(map[string]struct{},len(reports))
 for _,report:=range reports {
  if _,ok:=expected[report.NodeID];!ok{return fmt.Errorf("%w: unexpected node %q",ErrIncompleteNetworkAcknowledgement,report.NodeID)}
  if _,duplicate:=seen[report.NodeID];duplicate{return fmt.Errorf("%w: duplicate node %q",ErrIncompleteNetworkAcknowledgement,report.NodeID)}
  seen[report.NodeID]=struct{}{}
  if !report.Applied || report.SubscriptionID!=job.SubscriptionID ||
    report.PublicIPID!=job.PublicIPID || report.Revision!=job.Revision {
   return fmt.Errorf("%w: node %q has not applied expected revision",ErrIncompleteNetworkAcknowledgement,report.NodeID)
  }
 }
 if len(seen)!=len(expected){return ErrIncompleteNetworkAcknowledgement}
 return nil
}
