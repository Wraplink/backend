package starterpolicy

import (
 "errors"
 "time"
)

var (
 ErrAlreadyActivated = errors.New("starter plan already activated")
 ErrReservationExpired = errors.New("starter reservation expired")
 ErrInvalidDuration = errors.New("starter duration must be positive")
)

type ActivationWindow struct {
 StartsAt time.Time
 ExpiresAt time.Time
}

// Window computes the time window only for a reserved, unexpired claim.
// Persisting activation, subscription creation, and entitlement provisioning
// must happen atomically/idempotently in the application service.
func Window(now, reservationExpiresAt time.Time, duration time.Duration, state string) (ActivationWindow,error) {
 if duration<=0 { return ActivationWindow{},ErrInvalidDuration }
 if state!="reserved" { return ActivationWindow{},ErrAlreadyActivated }
 if reservationExpiresAt.IsZero() || !now.Before(reservationExpiresAt) {
  return ActivationWindow{},ErrReservationExpired
 }
 return ActivationWindow{StartsAt:now,ExpiresAt:now.Add(duration)},nil
}
