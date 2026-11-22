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
  released,err:=store.RequeueReconciledSync(context.Background(),e)
  if released || !errors.Is(err,ErrReconciliationEvidenceRequired) {
   t.Errorf("case %d: released=%v err=%v",i,released,err)
  }
 }
}
