package publicip

import (
 "context"
 "errors"
 "testing"

 "github.com/google/uuid"
)

var errNetworkNotReconciled=errors.New("required edge node has not acknowledged revision")

type rejectingReconciliationVerifier struct{ called bool }
func (v *rejectingReconciliationVerifier) VerifyReconciliation(_ context.Context,_ uuid.UUID,_ uuid.UUID,_ int64) error {
 v.called=true
 return errNetworkNotReconciled
}

func TestReconciliationReleaseRejectsInvalidIdentityBeforeDB(t *testing.T) {
 store:=Store{}
 id:=uuid.New()
 for _,tc:=range []struct{subscription,ip uuid.UUID;revision int64}{
  {},{subscription:id,revision:1},{ip:id,revision:1},{subscription:id,ip:id},
 }{
  released,err:=store.RequeueReconciledSync(context.Background(),nil,tc.subscription,tc.ip,tc.revision)
  if released||!errors.Is(err,ErrReconciliationEvidenceRequired){t.Fatalf("released=%v err=%v",released,err)}
 }
}

func TestReconciliationReleaseRequiresVerifierBeforeDB(t *testing.T){
 store:=Store{}
 released,err:=store.RequeueReconciledSync(context.Background(),nil,uuid.New(),uuid.New(),1)
 if released||!errors.Is(err,ErrNoReconciliationVerifier){t.Fatalf("released=%v err=%v",released,err)}
}

func TestReconciliationReleaseRejectsFailedVerificationBeforeDB(t *testing.T){
 store:=Store{}
 verifier:=&rejectingReconciliationVerifier{}
 released,err:=store.RequeueReconciledSync(context.Background(),verifier,uuid.New(),uuid.New(),4)
 if released||!verifier.called||!errors.Is(err,errNetworkNotReconciled){t.Fatalf("released=%v called=%v err=%v",released,verifier.called,err)}
}

func TestReconciliationReleaseCancelledBeforeVerification(t *testing.T){
 store:=Store{}
 verifier:=&rejectingReconciliationVerifier{}
 ctx,cancel:=context.WithCancel(context.Background());cancel()
 released,err:=store.RequeueReconciledSync(ctx,verifier,uuid.New(),uuid.New(),4)
 if released||verifier.called||!errors.Is(err,context.Canceled){t.Fatalf("released=%v called=%v err=%v",released,verifier.called,err)}
}

type cancellingReconciliationVerifier struct{cancel context.CancelFunc}
func (v cancellingReconciliationVerifier) VerifyReconciliation(_ context.Context,_ uuid.UUID,_ uuid.UUID,_ int64) error {
 v.cancel()
 return nil
}

func TestReconciliationReleaseCancelledAfterVerification(t *testing.T){
 store:=Store{}
 ctx,cancel:=context.WithCancel(context.Background())
 released,err:=store.RequeueReconciledSync(ctx,cancellingReconciliationVerifier{cancel},uuid.New(),uuid.New(),5)
 if released||!errors.Is(err,context.Canceled){t.Fatalf("released=%v err=%v",released,err)}
}
