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
 job:=SyncJob{SubscriptionID:uuid.New(),PublicIPID:uuid.New(),Revision:1,AttemptID:uuid.New()}
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
 job:=SyncJob{SubscriptionID:uuid.New(),PublicIPID:uuid.New(),Revision:3,AttemptID:uuid.New()}
 report:=NodeAcknowledgement{NodeID:"dns-local-01",SubscriptionID:job.SubscriptionID,PublicIPID:job.PublicIPID,Revision:3,AttemptID:job.AttemptID,Applied:true}
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

type changingNodeSource struct {
 calls int
 report NodeAcknowledgement
}
func (s *changingNodeSource) RequiredNodes(_ context.Context,_ SyncJob)([]string,error){
 s.calls++
 if s.calls==1{return []string{"dns-local-01"},nil}
 return []string{"dns-local-01","edge-eu-01"},nil
}
func (s *changingNodeSource) AppliedReports(_ context.Context,_ SyncJob)([]NodeAcknowledgement,error){
 return []NodeAcknowledgement{s.report},nil
}
func TestAcknowledgedApplierRejectsMembershipChanges(t *testing.T){
 job:=SyncJob{SubscriptionID:uuid.New(),PublicIPID:uuid.New(),Revision:9,AttemptID:uuid.New()}
 source:=&changingNodeSource{report:NodeAcknowledgement{
  NodeID:"dns-local-01",SubscriptionID:job.SubscriptionID,
  PublicIPID:job.PublicIPID,Revision:job.Revision,AttemptID:job.AttemptID,Applied:true,
 }}
 apply:=&stubNetworkApplier{}
 err:=(AcknowledgedEntitlementApplier{Apply:apply,Source:source}).ApplyPublicIP(context.Background(),job)
 if !errors.Is(err,ErrRequiredNodesChanged) || !apply.called || source.calls!=2{
  t.Fatalf("err=%v applied=%v membershipReads=%d",err,apply.called,source.calls)
 }
}
func TestSameRequiredNodes(t *testing.T){
 if !sameRequiredNodes([]string{"dns","edge"},[]string{"edge","dns"}){t.Fatal("order must not matter")}
 for _,nodes:=range [][]string{{"dns","dns"},{"dns","other"},{""},{"dns"}}{
  if sameRequiredNodes([]string{"dns","edge"},nodes){t.Fatalf("accepted %v",nodes)}
 }
}

type cancellingMembershipSource struct {
 cancel context.CancelFunc
 report NodeAcknowledgement
 calls int
}
func (s *cancellingMembershipSource) RequiredNodes(_ context.Context,_ SyncJob)([]string,error){
 s.calls++
 if s.calls==2{s.cancel()}
 return []string{"dns-local-01"},nil
}
func (s *cancellingMembershipSource) AppliedReports(_ context.Context,_ SyncJob)([]NodeAcknowledgement,error){
 return []NodeAcknowledgement{s.report},nil
}
func TestAcknowledgedApplierRejectsCancellationAfterMembershipCheck(t *testing.T){
 job:=SyncJob{SubscriptionID:uuid.New(),PublicIPID:uuid.New(),Revision:11,AttemptID:uuid.New()}
 ctx,cancel:=context.WithCancel(context.Background())
 source:=&cancellingMembershipSource{cancel:cancel,report:NodeAcknowledgement{
  NodeID:"dns-local-01",SubscriptionID:job.SubscriptionID,PublicIPID:job.PublicIPID,Revision:job.Revision,AttemptID:job.AttemptID,Applied:true,
 }}
 err:=(AcknowledgedEntitlementApplier{Apply:&stubNetworkApplier{},Source:source}).ApplyPublicIP(ctx,job)
 if !errors.Is(err,context.Canceled)||source.calls!=2{t.Fatalf("err=%v calls=%d",err,source.calls)}
}
