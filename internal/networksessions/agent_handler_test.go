package networksessions

import (
 "net/http"
 "net/http/httptest"
 "testing"
)

func TestAgentTransportRejectsMissingCredentials(t *testing.T) {
 h:=&AgentHandler{}
 cases:=[]struct{
  name string
  method string
  path string
  call func(http.ResponseWriter,*http.Request)
 }{
  {"report",http.MethodPost,"/report",h.Report},
  {"poll",http.MethodGet,"/commands",h.Commands},
  {"ack",http.MethodPost,"/commands/123/ack",h.Acknowledge},
 }
 for _,tc:=range cases {
  t.Run(tc.name,func(t *testing.T){
   request:=httptest.NewRequest(tc.method,tc.path,nil)
   recorder:=httptest.NewRecorder()
   tc.call(recorder,request)
   if recorder.Code!=http.StatusServiceUnavailable {
    t.Fatalf("missing repository must not accept requests: got %d",recorder.Code)
   }
  })
 }
}
