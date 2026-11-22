package publicip

import (
 "context"
 "errors"
 "testing"

 "github.com/google/uuid"
)

func TestReconciliationReleaseRejectsIncompleteEvidenceBeforeDB(t *testing.T) {
 store:=Store{} // Validation must fail without touching the database.
 valid:=ReconciliationEvidence{
  SubscriptionID:uuid.New(),PublicIPID:uuid.New(),Revision:2,
  OldAttemptFenced:true,RequiredNodesChecked:true,
 }
 cases:=[]ReconciliationEvidence{
  {},
  {PublicIPID:valid.PublicIPID,Revision:2,OldAttemptFenced:true,RequiredNodesChecked:true},
  {SubscriptionID:valid.SubscriptionID,Revision:2,OldAttemptFenced:true,RequiredNodesChecked:true},
  {SubscriptionID:valid.SubscriptionID,PublicIPID:valid.PublicIPID,OldAttemptFenced:true,RequiredNodesChecked:true},
  {SubscriptionID:valid.SubscriptionID,PublicIPID:valid.PublicIPID,Revision:2,RequiredNodesChecked:true},
  {SubscriptionID:valid.SubscriptionID,PublicIPID:valid.PublicIPID,Revision:2,OldAttemptFenced:true},
 }
 for i,e:=range cases {
  released,err:=store.RequeueReconciledSync(context.Background(),nil,e)
  if released || !errors.Is(err,ErrReconciliationEvidenceRequired) {
   t.Errorf("case %d: released=%v err=%v",i,released,err)
  }
 }
}

func TestReconciliationReleaseRequiresVerifierBeforeDB(t *testing.T) {
 store:=Store{}
 evidence:=ReconciliationEvidence{
  SubscriptionID:uuid.New(),PublicIPID:uuid.New(),Revision:1,
  OldAttemptFenced:true,RequiredNodesChecked:true,
 }
 released,err:=store.RequeueReconciledSync(context.Background(),nil,evidence)
 if released || !errors.Is(err,ErrNoReconciliationVerifier){
  t.Fatalf("released=%v err=%v; want missing verifier",released,err)
 }
}

type rejectingReconciliationVerifier struct { called bool }
func (v *rejectingReconciliationVerifier) VerifyReconciliation(_ context.Context,_ ReconciliationEvidence) error {
 v.called=true
 return errNetworkNotReconciled
}
var errNetworkNotReconciled=errors.New("required edge node has not acknowledged revision")

func TestReconciliationReleaseRejectsFailedVerificationBeforeDB(t *testing.T) {
 store:=Store{} // A rejected verification must never access the DB.
 verifier:=&rejectingReconciliationVerifier{}
 evidence:=ReconciliationEvidence{
  SubscriptionID:uuid.New(),PublicIPID:uuid.New(),Revision:4,
  OldAttemptFenced:true,RequiredNodesChecked:true,
 }
 released,err:=store.RequeueReconciledSync(context.Background(),verifier,evidence)
 if released || !verifier.called || !errors.Is(err,errNetworkNotReconciled){
  t.Fatalf("released=%v verified=%v err=%v",released,verifier.called,err)
 }
}

func TestReconciliationReleaseCancelledBeforeVerification(t *testing.T) {
 store:=Store{}
 verifier:=&rejectingReconciliationVerifier{}
 evidence:=ReconciliationEvidence{
  SubscriptionID:uuid.New(),PublicIPID:uuid.New(),Revision:4,
  OldAttemptFenced:true,RequiredNodesChecked:true,
 }
 ctx,cancel:=context.WithCancel(context.Background())
 cancel()
 released,err:=store.RequeueReconciledSync(ctx,verifier,evidence)
 if released || verifier.called || !errors.Is(err,context.Canceled) {
  t.Fatalf("released=%v verifierCalled=%v err=%v",released,verifier.called,err)
 }
}

type cancellingReconciliationVerifier struct { cancel context.CancelFunc }
func (v cancellingReconciliationVerifier) VerifyReconciliation(_ context.Context,_ ReconciliationEvidence) error {
 v.cancel()
 return nil
}

func TestReconciliationReleaseCancelledAfterVerification(t *testing.T) {
 store:=Store{} // Must not reach DB after verifier cancels context.
 ctx,cancel:=context.WithCancel(context.Background())
 evidence:=ReconciliationEvidence{
  SubscriptionID:uuid.New(),PublicIPID:uuid.New(),Revision:5,
  OldAttemptFenced:true,RequiredNodesChecked:true,
 }
 released,err:=store.RequeueReconciledSync(ctx,cancellingReconciliationVerifier{cancel:cancel},evidence)
 if released || !errors.Is(err,context.Canceled) {
  t.Fatalf("released=%v err=%v",released,err)
 }
}
