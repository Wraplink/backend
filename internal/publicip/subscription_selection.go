package publicip

import (
 "context"
 "errors"
 "fmt"

 "github.com/google/uuid"
 "github.com/jackc/pgx/v5"
)

// SubscriptionSelection reports the saved preference, not an active entitlement.
type SubscriptionSelection struct {
 SubscriptionID uuid.UUID `json:"subscriptionId"`
 PublicIPID *uuid.UUID `json:"publicIpId"`
 IPAddress *string `json:"ipAddress"`
 Verified bool `json:"verified"`
}

// GetSubscriptionSelection returns a preference only for a subscription owned
// by the caller. An absent preference is represented by null, not an error.
func (s Store) GetSubscriptionSelection(ctx context.Context,userID,subscriptionID uuid.UUID)(SubscriptionSelection,error){
 result:=SubscriptionSelection{SubscriptionID:subscriptionID}
 err:=s.Pool.QueryRow(ctx,`
  SELECT selection.public_ip_id, host(ip.ip_address),
         COALESCE(ip.verified_at IS NOT NULL,false)
  FROM user_subscriptions subscription
  LEFT JOIN subscription_public_ip_selections selection
    ON selection.subscription_id=subscription.id
  LEFT JOIN user_public_ips ip
    ON ip.id=selection.public_ip_id AND ip.user_id=subscription.user_id
       AND ip.removed_at IS NULL
  WHERE subscription.id=$1 AND subscription.user_id=$2
 `,subscriptionID,userID).Scan(&result.PublicIPID,&result.IPAddress,&result.Verified)
 if errors.Is(err,pgx.ErrNoRows){return SubscriptionSelection{},ErrSubscriptionNotOwned}
 if err!=nil{return SubscriptionSelection{},fmt.Errorf("read subscription IP selection: %w",err)}
 return result,nil
}
