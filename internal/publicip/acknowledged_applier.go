package publicip

import (
 "context"
 "errors"
 "fmt"
)

// AuthenticatedAcknowledgementSource must discover required DNS/edge nodes and
// return authenticated reports of their effective applied state. A heartbeat,
// policy receipt, or configuration download must not count as applied.
type AuthenticatedAcknowledgementSource interface {
 RequiredNodes(ctx context.Context, job SyncJob) ([]string,error)
 AppliedReports(ctx context.Context, job SyncJob) ([]NodeAcknowledgement,error)
}

var ErrNoAcknowledgementSource=errors.New("authenticated network acknowledgement source is not configured")
var ErrRequiredNodesChanged=errors.New("required network node membership changed during application")

func sameRequiredNodes(before,after []string) bool {
 if len(before)!=len(after){return false}
 nodes:=make(map[string]struct{},len(before))
 for _,node:=range before {if node==""{return false};if _,ok:=nodes[node];ok{return false};nodes[node]=struct{}{}}
 for _,node:=range after {if _,ok:=nodes[node];!ok{return false};delete(nodes,node)}
 return len(nodes)==0
}

// AcknowledgedEntitlementApplier delegates application to the actual control
// plane and validates its authenticated node acknowledgements before allowing
// the worker to record a successful revision. This adapter alone does not
// implement network application, authentication, or durable node fencing.
type AcknowledgedEntitlementApplier struct {
 Apply EntitlementApplier
 Source AuthenticatedAcknowledgementSource
}

func (a AcknowledgedEntitlementApplier) ApplyPublicIP(ctx context.Context,job SyncJob) error {
 if a.Apply==nil{return ErrNoEntitlementApplier}
 if a.Source==nil{return ErrNoAcknowledgementSource}
 if err:=ctx.Err();err!=nil{return err}
 required,err:=a.Source.RequiredNodes(ctx,job)
 if err!=nil{return fmt.Errorf("discover required nodes: %w",err)}
 if len(required)==0{return ErrIncompleteNetworkAcknowledgement}
 if err:=a.Apply.ApplyPublicIP(ctx,job);err!=nil{return fmt.Errorf("apply network entitlement: %w",err)}
 if err:=ctx.Err();err!=nil{return err}
 reports,err:=a.Source.AppliedReports(ctx,job)
 if err!=nil{return fmt.Errorf("read applied node reports: %w",err)}
 if err:=ValidateNetworkAcknowledgements(job,required,reports);err!=nil{return err}
 // Re-read membership after applying. A changing required-node set cannot
 // be treated as a successful acknowledgement snapshot.
 current,err:=a.Source.RequiredNodes(ctx,job)
 if err!=nil{return fmt.Errorf("recheck required nodes: %w",err)}
 if !sameRequiredNodes(required,current){return ErrRequiredNodesChanged}
 return nil
}
