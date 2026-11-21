package publicip

import (
 "context"
 "errors"
 "testing"
 "time"
)

func TestReconcileOneFailsClosedWithoutApplier(t *testing.T) {
 var store Store // A missing applier must not touch the database.
 processed, err := store.ReconcileOne(context.Background(), nil)
 if processed || !errors.Is(err, ErrNoEntitlementApplier) {
  t.Fatalf("processed=%v err=%v; want missing applier", processed, err)
 }
}

func TestStaleSyncRejectsUnsafeTimeout(t *testing.T){
 var store Store // Invalid durations must be rejected before DB access.
 for _,timeout:=range []time.Duration{0,-time.Second,30*time.Second}{
  count,err:=store.RequeueStaleSync(context.Background(),timeout)
  if err==nil || count!=0{t.Fatalf("timeout=%v count=%d err=%v",timeout,count,err)}
 }
}
