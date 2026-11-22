package publicip

import (
 "errors"
 "testing"

 "github.com/google/uuid"
)

func TestValidateNetworkAcknowledgements(t *testing.T) {
 job:=SyncJob{SubscriptionID:uuid.New(),PublicIPID:uuid.New(),Revision:7,AttemptID:uuid.New()}
 dns:=NodeAcknowledgement{NodeID:"dns-local-01",SubscriptionID:job.SubscriptionID,PublicIPID:job.PublicIPID,Revision:job.Revision,AttemptID:job.AttemptID,Applied:true}
 edge:=dns
 edge.NodeID="edge-eu-01"
 required:=[]string{"dns-local-01","edge-eu-01"}
 cases:=[]struct{
  name string
  required []string
  reports []NodeAcknowledgement
  wantError bool
 }{
  {"all applied",required,[]NodeAcknowledgement{edge,dns},false},
  {"missing edge",required,[]NodeAcknowledgement{dns},true},
  {"no required nodes",nil,[]NodeAcknowledgement{dns},true},
  {"duplicate required node",[]string{"dns-local-01","dns-local-01"},[]NodeAcknowledgement{dns},true},
  {"duplicate report",required,[]NodeAcknowledgement{dns,dns,edge},true},
  {"unexpected node",[]string{"dns-local-01"},[]NodeAcknowledgement{dns,edge},true},
  {"received but not applied",required,[]NodeAcknowledgement{dns,func()NodeAcknowledgement{x:=edge;x.Applied=false;return x}()},true},
  {"stale revision",required,[]NodeAcknowledgement{dns,func()NodeAcknowledgement{x:=edge;x.Revision--;return x}()},true},
  {"future revision",required,[]NodeAcknowledgement{dns,func()NodeAcknowledgement{x:=edge;x.Revision++;return x}()},true},
  {"wrong subscription",required,[]NodeAcknowledgement{dns,func()NodeAcknowledgement{x:=edge;x.SubscriptionID=uuid.New();return x}()},true},
  {"wrong attempt",required,[]NodeAcknowledgement{dns,func()NodeAcknowledgement{x:=edge;x.AttemptID=uuid.New();return x}()},true},
  {"wrong IP",required,[]NodeAcknowledgement{dns,func()NodeAcknowledgement{x:=edge;x.PublicIPID=uuid.New();return x}()},true},
 }
 for _,tc:=range cases {
  t.Run(tc.name,func(t *testing.T){
   err:=ValidateNetworkAcknowledgements(job,tc.required,tc.reports)
   if tc.wantError && !errors.Is(err,ErrIncompleteNetworkAcknowledgement){t.Fatalf("got %v, want incomplete acknowledgement",err)}
   if !tc.wantError && err!=nil{t.Fatal(err)}
  })
 }
}

func TestValidateNetworkAcknowledgementsRequiresClaimedAttempt(t *testing.T) {
 job:=SyncJob{SubscriptionID:uuid.New(),PublicIPID:uuid.New(),Revision:12}
 report:=NodeAcknowledgement{
  NodeID:"dns-local-01",SubscriptionID:job.SubscriptionID,
  PublicIPID:job.PublicIPID,Revision:job.Revision,
  AttemptID:uuid.New(),Applied:true,
 }
 err:=ValidateNetworkAcknowledgements(job,[]string{"dns-local-01"},[]NodeAcknowledgement{report})
 if !errors.Is(err,ErrIncompleteNetworkAcknowledgement){
  t.Fatalf("unclaimed attempt accepted: %v",err)
 }
}
