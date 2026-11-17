package dnsregistry

import (
 "context"
 "net/http"
 "net/http/httptest"
 "os"
 "testing"
 "time"

 "github.com/go-chi/chi/v5"
 "github.com/jackc/pgx/v5/pgxpool"
)

func TestAdminRemoveEdgePostgres(t *testing.T) {
 url:=os.Getenv("TEST_DATABASE_URL")
 if url=="" {t.Skip("TEST_DATABASE_URL not set")}
 ctx,cancel:=context.WithTimeout(context.Background(),15*time.Second)
 defer cancel()
 db,err:=pgxpool.New(ctx,url)
 if err!=nil {t.Fatal(err)}
 defer db.Close()
 id:="test-remove-edge-"+time.Now().UTC().Format("150405.000000000")
 _,err=db.Exec(ctx,`INSERT INTO network_agent_health(id,region,sni_capacity,route_capacity,status,registered_at,last_seen_at,revision)
 VALUES($1,'test',1,0,'online',now(),now()-interval '5 minutes',nextval('network_agent_health_revision_seq'))`,id)
 if err!=nil {t.Fatal(err)}
 defer db.Exec(context.Background(),"DELETE FROM network_agent_health WHERE id=$1",id)
 h:=NewHandler(NewStore(),"test",time.Minute,db)
 router:=chi.NewRouter()
 router.Delete("/nodes/{type}/{id}",h.AdminRemoveNode)
 remove:=func()int{
  t.Helper()
  rec:=httptest.NewRecorder()
  router.ServeHTTP(rec,httptest.NewRequest(http.MethodDelete,"/nodes/edge/"+id,nil))
  return rec.Code
 }
 // Row lock ensures that a concurrent heartbeat wins before the deletion
 // checks last_seen_at; a refreshed node must not be removed.
 tx,err:=db.Begin(ctx)
 if err!=nil {t.Fatal(err)}
 if _,err=tx.Exec(ctx,"UPDATE network_agent_health SET last_seen_at=now(),status='online' WHERE id=$1",id);err!=nil {tx.Rollback(ctx);t.Fatal(err)}
 done:=make(chan int,1)
 go func(){
  rec:=httptest.NewRecorder()
  router.ServeHTTP(rec,httptest.NewRequest(http.MethodDelete,"/nodes/edge/"+id,nil))
  done<-rec.Code
 }()
 if err=tx.Commit(ctx);err!=nil {t.Fatal(err)}
 if status:=<-done;status!=http.StatusConflict {t.Fatalf("refreshed node delete status=%d want 409",status)}
 var count int
 if err=db.QueryRow(ctx,"SELECT count(*) FROM network_agent_health WHERE id=$1",id).Scan(&count);err!=nil||count!=1 {t.Fatalf("refreshed node missing: count=%d err=%v",count,err)}
 if _,err=db.Exec(ctx,"UPDATE network_agent_health SET last_seen_at=now()-interval '5 minutes' WHERE id=$1",id);err!=nil {t.Fatal(err)}
 if status:=remove();status!=http.StatusNoContent {t.Fatalf("stale node delete status=%d want 204",status)}
 if status:=remove();status!=http.StatusNotFound {t.Fatalf("missing node delete status=%d want 404",status)}
 // A stale agent with route IP inventory must not be deleted, even when
 // its inventory has been disabled. Keep the inventory and health row intact.
 if _,err=db.Exec(ctx,`INSERT INTO network_agent_health(id,region,sni_capacity,route_capacity,status,registered_at,last_seen_at,revision)
 VALUES($1,'test',1,1,'online',now(),now()-interval '5 minutes',nextval('network_agent_health_revision_seq'))`,id);err!=nil {t.Fatal(err)}
 _,err=db.Exec(ctx,"INSERT INTO network_route_agent_ips(agent_id,public_ip,enabled,last_seen_at) VALUES($1,$2::inet,false,now())",id,"198.51.100.245")
 if err!=nil {t.Fatal(err)}
 defer db.Exec(context.Background(),"DELETE FROM network_route_agent_ips WHERE agent_id=$1",id)
 if status:=remove();status!=http.StatusConflict {t.Fatalf("inventory-protected node status=%d want 409",status)}
 if err=db.QueryRow(ctx,"SELECT count(*) FROM network_agent_health WHERE id=$1",id).Scan(&count);err!=nil||count!=1 {t.Fatalf("protected node missing: count=%d err=%v",count,err)}
 if err=db.QueryRow(ctx,"SELECT count(*) FROM network_route_agent_ips WHERE agent_id=$1",id).Scan(&count);err!=nil||count!=1 {t.Fatalf("inventory missing: count=%d err=%v",count,err)}

}
