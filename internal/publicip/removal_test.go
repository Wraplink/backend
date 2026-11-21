package publicip

import (
 "errors"
 "testing"
)

func TestCanRemove(t *testing.T) {
 cases := []struct{
  name string
  owned, removed bool
  active int
  want error
 }{
  {"inactive owned", true, false, 0, nil},
  {"active owned", true, false, 1, ErrActiveAddress},
  {"shared across subscriptions", true, false, 2, ErrActiveAddress},
  {"other user", false, false, 0, ErrNotOwned},
  {"already removed", true, true, 0, ErrNotOwned},
 }
 for _, tc := range cases {
  t.Run(tc.name, func(t *testing.T) {
   got := CanRemove(tc.owned, tc.removed, tc.active)
   if !errors.Is(got, tc.want) {
    t.Fatalf("CanRemove() = %v, want %v", got, tc.want)
   }
  })
 }
}
