package starterpolicy

import "time"

// DisplayClaimState derives the visible starter state from both the durable
// claim and its linked subscription. It never grants network entitlement.
func DisplayClaimState(state string, reservationExpiresAt, startsAt, expiresAt *time.Time, subscriptionStatus *string, now time.Time) string {
 if state=="reserved" {
  if reservationExpiresAt==nil || !now.Before(*reservationExpiresAt) {return "expired"}
  return state
 }
 if state!="active" {return state}
 if subscriptionStatus==nil || *subscriptionStatus!="active" {return "cancelled"}
 if startsAt==nil || expiresAt==nil || startsAt.After(now) || !expiresAt.After(*startsAt) {return "failed"}
 if !now.Before(*expiresAt) {return "expired"}
 return "active"
}
