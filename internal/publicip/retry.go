package publicip

import (
 "context"
 "fmt"
 "time"

 "github.com/google/uuid"
)

// RequeueStaleSync reclaims jobs abandoned by a crashed worker. This operation
// is safe only when downstream entitlement application is idempotent and a
// successful apply is confirmed before AckSync is called.
func (s Store) RequeueStaleSync(ctx context.Context, timeout time.Duration) (int64,error) {
 if timeout < time.Minute {return 0,fmt.Errorf("sync timeout must be at least one minute")}
 tag,err:=s.Pool.Exec(ctx,`
  UPDATE subscription_public_ip_sync
  SET status='pending', updated_at=now(),
      last_error='previous sync attempt expired; awaiting reconciliation'
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
  SET status='pending',last_error=NULL,updated_at=now()
  WHERE subscription_id=$1 AND revision=$2 AND status='failed'
 `,subscriptionID,revision)
 if err!=nil{return false,fmt.Errorf("retry failed IP sync: %w",err)}
 return tag.RowsAffected()==1,nil
}
