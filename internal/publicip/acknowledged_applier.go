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
 return nil
}
