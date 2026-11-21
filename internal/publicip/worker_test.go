package publicip

import (
 "context"
 "errors"
 "testing"
)

func TestReconcileOneFailsClosedWithoutApplier(t *testing.T) {
 var store Store // A missing applier must not touch the database.
 processed, err := store.ReconcileOne(context.Background(), nil)
 if processed || !errors.Is(err, ErrNoEntitlementApplier) {
  t.Fatalf("processed=%v err=%v; want missing applier", processed, err)
 }
}
