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
 if now.Add(duration).Before(now) { return ActivationWindow{},ErrInvalidDuration }
 if reservationExpiresAt.IsZero() || !now.Before(reservationExpiresAt) {
  return ActivationWindow{},ErrReservationExpired
 }
 return ActivationWindow{StartsAt:now,ExpiresAt:now.Add(duration)},nil
}

// DurationFromSeconds converts persisted policy seconds without overflowing
// Go's nanosecond-based time.Duration representation.
func DurationFromSeconds(seconds int64) (time.Duration, error) {
 const maxSeconds = int64((1<<63 - 1) / int64(time.Second))
 if seconds <= 0 || seconds > maxSeconds {
  return 0, ErrInvalidDuration
 }
 return time.Duration(seconds) * time.Second, nil
}
