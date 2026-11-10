package starterpolicy

import (
 "context"
 "github.com/google/uuid"
 "github.com/jackc/pgx/v5/pgxpool"
)

type FreeClaimSummary struct {
 PlanID uuid.UUID `json:"planId"`
 PlanCode string `json:"planCode"`
 Source string `json:"source"`
 State string `json:"state"`
}
func ListFreeClaims(ctx context.Context,db *pgxpool.Pool,userID uuid.UUID)([]FreeClaimSummary,error){
 rows,err:=db.Query(ctx,`SELECT c.plan_id,COALESCE(c.plan_code,p.code,''),c.source,c.state
 FROM free_plan_claims c LEFT JOIN plans p ON p.id=c.plan_id
 WHERE c.user_id=$1 ORDER BY c.assigned_at DESC`,userID)
 if err!=nil{return nil,err}
 defer rows.Close()
 items:=make([]FreeClaimSummary,0)
 for rows.Next(){
  var item FreeClaimSummary
  if err:=rows.Scan(&item.PlanID,&item.PlanCode,&item.Source,&item.State);err!=nil{return nil,err}
  items=append(items,item)
 }
 return items,rows.Err()
}
