package publicip

import (
 "context"
 "fmt"
 "time"

 "github.com/google/uuid"
)

// RequeueStaleSync quarantines expired applying jobs as failed instead of
// automatically replaying an operation with uncertain external side effects.
// A running downstream operation may still finish after its lease expires;
// the control plane must enforce revision fencing before retry is enabled.
func (s Store) RequeueStaleSync(ctx context.Context, timeout time.Duration) (int64,error) {
 if timeout < time.Minute {return 0,fmt.Errorf("sync timeout must be at least one minute")}
 tag,err:=s.Pool.Exec(ctx,`
  UPDATE subscription_public_ip_sync
  SET status='needs_reconciliation', attempt_id=NULL, updated_at=now(),
      last_error='sync attempt expired; verify downstream state before retry'
  WHERE status='applying' AND updated_at < now()-($1::bigint * interval '1 microsecond')
 `,timeout.Microseconds())
 if err!=nil{return 0,fmt.Errorf("requeue stale IP sync jobs: %w",err)}
 return tag.RowsAffected(),nil
}

// RetryFailedSync requeues a specific failed revision. Revision matching
// prevents retrying an old request after the user changes the selected IP.
func (s Store) RetryFailedSync(ctx context.Context,subscriptionID uuid.UUID,revision int64)(bool,error){
 if revision<1 {return false,fmt.Errorf("invalid sync revision")}
 tag,err:=s.Pool.Exec(ctx,`
  UPDATE subscription_public_ip_sync
  SET status='pending',attempt_id=NULL,last_error=NULL,updated_at=now()
  WHERE subscription_id=$1 AND revision=$2 AND status='failed'

 `,subscriptionID,revision)
 if err!=nil{return false,fmt.Errorf("retry failed IP sync: %w",err)}
 return tag.RowsAffected()==1,nil
}

// RetryFailedSyncForUser retries only a failed revision owned by the
// authenticated user. The revision guard prevents stale browser requests
// from requeuing a newer selection.
func (s Store) RetryFailedSyncForUser(ctx context.Context,userID,subscriptionID uuid.UUID,revision int64)(bool,error){
 if revision<1{return false,fmt.Errorf("invalid sync revision")}
 tag,err:=s.Pool.Exec(ctx,`
  UPDATE subscription_public_ip_sync AS sync
  SET status='pending',attempt_id=NULL,last_error=NULL,updated_at=now()
  FROM user_subscriptions AS subscription
  WHERE sync.subscription_id=subscription.id
    AND subscription.id=$1 AND subscription.user_id=$2
    AND sync.revision=$3 AND sync.status='failed'

 `,subscriptionID,userID,revision)
 if err!=nil{return false,fmt.Errorf("retry owned IP sync: %w",err)}
 return tag.RowsAffected()==1,nil
}
