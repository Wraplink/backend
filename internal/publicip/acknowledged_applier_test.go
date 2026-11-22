package publicip

import (
 "context"
 "errors"
 "testing"

 "github.com/google/uuid"
)

type stubNetworkApplier struct{ called bool }
func (s *stubNetworkApplier) ApplyPublicIP(_ context.Context,_ SyncJob) error {s.called=true;return nil}

type stubAcknowledgementSource struct{
 required []string
 reports []NodeAcknowledgement
}
func (s stubAcknowledgementSource) RequiredNodes(_ context.Context,_ SyncJob)([]string,error){return s.required,nil}
func (s stubAcknowledgementSource) AppliedReports(_ context.Context,_ SyncJob)([]NodeAcknowledgement,error){return s.reports,nil}

func TestAcknowledgedApplierFailsClosed(t *testing.T){
 job:=SyncJob{SubscriptionID:uuid.New(),PublicIPID:uuid.New(),Revision:1}
 apply:=&stubNetworkApplier{}
 if err:=(AcknowledgedEntitlementApplier{Apply:apply}).ApplyPublicIP(context.Background(),job);!errors.Is(err,ErrNoAcknowledgementSource){
  t.Fatalf("missing source: %v",err)
 }
 if apply.called{t.Fatal("applier ran without acknowledgement source")}
 if err:=(AcknowledgedEntitlementApplier{Source:stubAcknowledgementSource{}}).ApplyPublicIP(context.Background(),job);!errors.Is(err,ErrNoEntitlementApplier){
  t.Fatalf("missing applier: %v",err)
 }
 if err:=(AcknowledgedEntitlementApplier{Apply:apply,Source:stubAcknowledgementSource{}}).ApplyPublicIP(context.Background(),job);!errors.Is(err,ErrIncompleteNetworkAcknowledgement){
  t.Fatalf("missing required nodes: %v",err)
 }
 if apply.called{t.Fatal("applier ran without required nodes")}
}

func TestAcknowledgedApplierRequiresExactAppliedReports(t *testing.T){
 job:=SyncJob{SubscriptionID:uuid.New(),PublicIPID:uuid.New(),Revision:3}
 report:=NodeAcknowledgement{NodeID:"dns-local-01",SubscriptionID:job.SubscriptionID,PublicIPID:job.PublicIPID,Revision:3,Applied:true}
 apply:=&stubNetworkApplier{}
 source:=stubAcknowledgementSource{required:[]string{"dns-local-01","edge-eu-01"},reports:[]NodeAcknowledgement{report}}
 adapter:=AcknowledgedEntitlementApplier{Apply:apply,Source:source}
 if err:=adapter.ApplyPublicIP(context.Background(),job);!errors.Is(err,ErrIncompleteNetworkAcknowledgement){
  t.Fatalf("missing edge acknowledgement: %v",err)
 }
 if !apply.called{t.Fatal("network applier not called")}
 edge:=report;edge.NodeID="edge-eu-01"
 source.reports=append(source.reports,edge)
 adapter.Source=source
 if err:=adapter.ApplyPublicIP(context.Background(),job);err!=nil{t.Fatalf("all nodes applied: %v",err)}
}
