package publicip

import "errors"

var (
 ErrNotOwned = errors.New("public IP address not found")
 ErrActiveAddress = errors.New("activate a different verified address before removing this IP")
)

// CanRemove decides whether an address can be soft-deleted.
// Callers must load the address and its active selection count in one transaction
// while locking the address row, to prevent concurrent activation/deletion.
func CanRemove(owned, alreadyRemoved bool, activeSubscriptions int) error {
 if !owned || alreadyRemoved {
  return ErrNotOwned
 }
 if activeSubscriptions > 0 {
  return ErrActiveAddress
 }
 return nil
}
