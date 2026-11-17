package dnsregistry

import (
 "net/http"
 "net/http/httptest"
 "testing"
 "time"

 "github.com/go-chi/chi/v5"
)

func TestAdminRemoveDNSNode(t *testing.T) {
 tests:=[]struct{name string;id string;age time.Duration;want int}{
  {"missing","missing",0,http.StatusNotFound},
  {"online","dns-01",0,http.StatusConflict},
  {"stale","dns-01",2*time.Minute,http.StatusNoContent},
 }
 for _,tt:=range tests {
  t.Run(tt.name,func(t *testing.T){
   store:=NewStore()
   store.Upsert("dns-01","local",[]string{"192.0.2.10"})
   if tt.age>0 {
    store.mu.Lock()
    node:=store.nodes["dns-01"]
    node.LastSeenAt=time.Now().UTC().Add(-tt.age)
    store.nodes["dns-01"]=node
    store.mu.Unlock()
   }
   handler:=NewHandler(store,"test-key",time.Minute)
   router:=chi.NewRouter()
   router.Delete("/nodes/{type}/{id}",handler.AdminRemoveNode)
   req:=httptest.NewRequest(http.MethodDelete,"/nodes/dns/"+tt.id,nil)
   recorder:=httptest.NewRecorder()
   router.ServeHTTP(recorder,req)
   if recorder.Code!=tt.want {t.Fatalf("status=%d want=%d body=%s",recorder.Code,tt.want,recorder.Body.String())}
   if tt.want==http.StatusNoContent && len(store.List(0))!=0 {t.Fatal("stale node not removed")}
  })
 }
}

func TestAdminRemoveNodeRejectsInvalidType(t *testing.T) {
 handler:=NewHandler(NewStore(),"test-key",time.Minute)
 router:=chi.NewRouter()
 router.Delete("/nodes/{type}/{id}",handler.AdminRemoveNode)
 recorder:=httptest.NewRecorder()
 router.ServeHTTP(recorder,httptest.NewRequest(http.MethodDelete,"/nodes/unknown/test",nil))
 if recorder.Code!=http.StatusBadRequest {t.Fatalf("got %d",recorder.Code)}
}
