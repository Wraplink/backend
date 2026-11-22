package publicip

import (
 "context"
 "errors"
 "fmt"

 "github.com/google/uuid"
 "github.com/jackc/pgx/v5"
)

// SyncJob is a desired entitlement revision. The reconciler must apply the
// desired address to DNS Control and every required Edge Agent before AckSync.
type SyncJob struct {
 SubscriptionID uuid.UUID
 PublicIPID uuid.UUID
 IPAddress string
 Revision int64
 AttemptID uuid.UUID
}

// ClaimSync reserves one pending job. A separate lease/retry mechanism is
// required before using this with multiple workers in production.
func (s Store) ClaimSync(ctx context.Context) (SyncJob, bool, error) {
 tx,err:=s.Pool.BeginTx(ctx,pgx.TxOptions{})
 if err!=nil{return SyncJob{},false,fmt.Errorf("begin IP sync claim: %w",err)}
 defer tx.Rollback(ctx)
 var job SyncJob
 err=tx.QueryRow(ctx,`
  SELECT sync.subscription_id, sync.desired_public_ip_id,
         host(ip.ip_address), sync.revision
  FROM subscription_public_ip_sync sync
  JOIN user_public_ips ip ON ip.id=sync.desired_public_ip_id
  JOIN subscription_public_ip_selections selection
    ON selection.subscription_id=sync.subscription_id
   AND selection.public_ip_id=sync.desired_public_ip_id
   AND selection.user_id=ip.user_id
  JOIN user_subscriptions subscription
    ON subscription.id=sync.subscription_id
   AND subscription.user_id=ip.user_id
  WHERE sync.status='pending' AND ip.removed_at IS NULL
    AND ip.verified_at IS NOT NULL
  ORDER BY sync.updated_at
  LIMIT 1
  FOR UPDATE OF sync SKIP LOCKED
 `).Scan(&job.SubscriptionID,&job.PublicIPID,&job.IPAddress,&job.Revision)
 if errors.Is(err,pgx.ErrNoRows){return SyncJob{},false,nil}
 if err!=nil{return SyncJob{},false,fmt.Errorf("select IP sync job: %w",err)}
 job.AttemptID=uuid.New()
 _,err=tx.Exec(ctx,`
  UPDATE subscription_public_ip_sync
  SET status='applying', attempt_id=$3, updated_at=now()
  WHERE subscription_id=$1 AND revision=$2
 `,job.SubscriptionID,job.Revision,job.AttemptID)
 if err!=nil{return SyncJob{},false,fmt.Errorf("claim IP sync job: %w",err)}
 if err=tx.Commit(ctx);err!=nil{return SyncJob{},false,fmt.Errorf("commit IP sync claim: %w",err)}
 return job,true,nil
}

// AckSync acknowledges exactly the revision that was applied. An outdated
// worker cannot mark a newer selection applied.
func (s Store) AckSync(ctx context.Context,job SyncJob) (bool,error) {
 tag,err:=s.Pool.Exec(ctx,`
  UPDATE subscription_public_ip_sync
  SET applied_revision=$3,applied_public_ip_id=$2,status='applied',attempt_id=NULL,last_error=NULL,updated_at=now()
  WHERE subscription_id=$1 AND desired_public_ip_id=$2
    AND revision=$3 AND status='applying' AND attempt_id=$4
    AND EXISTS (
      SELECT 1 FROM subscription_public_ip_selections selection
      JOIN user_public_ips ip ON ip.id=selection.public_ip_id
      JOIN user_subscriptions subscription ON subscription.id=selection.subscription_id
      WHERE selection.subscription_id=$1 AND selection.public_ip_id=$2
        AND selection.user_id=subscription.user_id AND ip.user_id=subscription.user_id
        AND ip.removed_at IS NULL AND ip.verified_at IS NOT NULL
    )
 `,job.SubscriptionID,job.PublicIPID,job.Revision,job.AttemptID)
 if err!=nil{return false,fmt.Errorf("acknowledge IP sync: %w",err)}
 return tag.RowsAffected()==1,nil
}

// FailSync records an error without overwriting a superseding revision.
// Retries must be scheduled by a separate reconciliation process.
func (s Store) FailSync(ctx context.Context,job SyncJob,reason string)(bool,error){
 if len(reason)>1024 {reason=reason[:1024]}
 tag,err:=s.Pool.Exec(ctx,`
  UPDATE subscription_public_ip_sync
  SET status='failed',attempt_id=NULL,last_error=$4,updated_at=now()
  WHERE subscription_id=$1 AND desired_public_ip_id=$2
    AND revision=$3 AND status='applying' AND attempt_id=$5
 `,job.SubscriptionID,job.PublicIPID,job.Revision,reason,job.AttemptID)
 if err!=nil{return false,fmt.Errorf("fail IP sync: %w",err)}
 return tag.RowsAffected()==1,nil
}
