package dnsdomains

import (
 "context"
 "errors"
 "github.com/jackc/pgx/v5"
 "github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("managed DNS domain not found")
type Store struct { DB *pgxpool.Pool }
type Domain struct { ID string `json:"id"`; DomainInput }
const columns = "id::text, domain, enabled, mode, agent_id::text, host(override_ip), fallback_mode, priority"
func scanDomain(row pgx.Row) (Domain,error) {
 var d Domain
 err:=row.Scan(&d.ID,&d.DomainInput.Domain,&d.Enabled,&d.Mode,&d.AgentID,&d.OverrideIP,&d.FallbackMode,&d.Priority)
 return d,err
}
func (s Store) List(ctx context.Context) ([]Domain,error) {
 rows,err:=s.DB.Query(ctx,"SELECT "+columns+" FROM dns_managed_domains ORDER BY priority DESC, domain ASC")
 if err!=nil{return nil,err}
 defer rows.Close()
 result:=make([]Domain,0)
 for rows.Next(){d,err:=scanDomain(rows);if err!=nil{return nil,err};result=append(result,d)}
 return result,rows.Err()
}
func (s Store) Create(ctx context.Context,d DomainInput)(Domain,error){
 return scanDomain(s.DB.QueryRow(ctx,`INSERT INTO dns_managed_domains (domain,enabled,mode,agent_id,override_ip,fallback_mode,priority)
 VALUES ($1,$2,$3,$4,$5::inet,$6,$7) RETURNING `+columns,d.Domain,d.Enabled,d.Mode,d.AgentID,d.OverrideIP,d.FallbackMode,d.Priority))
}
func (s Store) Update(ctx context.Context,id string,d DomainInput)(Domain,error){
 result,err:=scanDomain(s.DB.QueryRow(ctx,`UPDATE dns_managed_domains SET domain=$2,enabled=$3,mode=$4,agent_id=$5,override_ip=$6::inet,fallback_mode=$7,priority=$8,updated_at=now()
 WHERE id=$1::uuid RETURNING `+columns,id,d.Domain,d.Enabled,d.Mode,d.AgentID,d.OverrideIP,d.FallbackMode,d.Priority))
 if errors.Is(err,pgx.ErrNoRows){return Domain{},ErrNotFound};return result,err
}
func (s Store) Delete(ctx context.Context,id string)error{
 tag,err:=s.DB.Exec(ctx,"DELETE FROM dns_managed_domains WHERE id=$1::uuid",id)
 if err!=nil{return err};if tag.RowsAffected()==0{return ErrNotFound};return nil
}
