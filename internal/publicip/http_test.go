package publicip

import (
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "net/netip"
 "strings"
 "testing"

 "github.com/google/uuid"
)

func TestHTTPRequiresAuthenticatedUser(t *testing.T) {
 h := Handler{AuthenticatedUserID: func(*http.Request)(uuid.UUID,bool){
  return uuid.Nil,false
 }}
 for _,tc:=range []struct{method,path string}{
  {"GET","/"},{"POST","/"},{"GET","/history"},{"GET","/observed"},
  {"PATCH","/"+uuid.NewString()},{"DELETE","/"+uuid.NewString()},
  {"POST","/"+uuid.NewString()+"/verify"},
  {"GET","/subscriptions/"+uuid.NewString()+"/sync"},
  {"GET","/subscriptions/"+uuid.NewString()+"/selection"},
  {"POST","/subscriptions/"+uuid.NewString()+"/sync/retry"},
 }{
  r:=httptest.NewRequest(tc.method,tc.path,strings.NewReader("{}"))
  w:=httptest.NewRecorder()
  h.Routes().ServeHTTP(w,r)
  if w.Code!=http.StatusUnauthorized {
   t.Errorf("%s %s: got %d, want 401",tc.method,tc.path,w.Code)
  }
 }
}

func TestHTTPObservedTrustedProxy(t *testing.T) {
 h:=Handler{
  AuthenticatedUserID:func(*http.Request)(uuid.UUID,bool){return uuid.New(),true},
  TrustedProxyPrefixes:[]netip.Prefix{netip.MustParsePrefix("172.20.0.0/16")},
 }
 r:=httptest.NewRequest(http.MethodGet,"/observed",nil)
 r.RemoteAddr="172.20.0.2:55000"
 r.Header.Set("X-Real-IP","8.8.8.8")
 w:=httptest.NewRecorder()
 h.Routes().ServeHTTP(w,r)
 if w.Code!=http.StatusOK || !strings.Contains(w.Body.String(),"8.8.8.8"){
  t.Fatalf("status=%d body=%s",w.Code,w.Body.String())
 }
}

func TestHTTPObservedRejectsSpoofedPrivatePeer(t *testing.T) {
 h:=Handler{AuthenticatedUserID:func(*http.Request)(uuid.UUID,bool){return uuid.New(),true}}
 r:=httptest.NewRequest(http.MethodGet,"/observed",nil)
 r.RemoteAddr="10.0.0.2:55000"
 r.Header.Set("X-Real-IP","8.8.8.8")
 w:=httptest.NewRecorder()
 h.Routes().ServeHTTP(w,r)
 if w.Code!=http.StatusUnprocessableEntity{
  t.Fatalf("status=%d want 422",w.Code)
 }
}

func TestRetrySyncRejectsInvalidRevisionBeforeDatabase(t *testing.T){
 h:=Handler{AuthenticatedUserID:func(*http.Request)(uuid.UUID,bool){return uuid.New(),true}}
 for _,body:=range []string{`{"revision":0}`,`{"revision":-1}`,`{"revision":"invalid"}`}{
  req:=httptest.NewRequest(http.MethodPost,"/subscriptions/"+uuid.NewString()+"/sync/retry",strings.NewReader(body))
  rec:=httptest.NewRecorder()
  h.Routes().ServeHTTP(rec,req)
  if rec.Code!=http.StatusBadRequest{t.Errorf("body=%s status=%d want 400",body,rec.Code)}
 }
}

func TestPublicIPDomainErrorStatus(t *testing.T){
 for _,tc:=range []struct{err error;status int}{
  {ErrNotOwned,http.StatusNotFound},
  {ErrSubscriptionNotOwned,http.StatusNotFound},
  {ErrUnverified,http.StatusConflict},
  {ErrActiveAddress,http.StatusConflict},
  {ErrSyncInProgress,http.StatusConflict},
 }{
  w:=httptest.NewRecorder()
  writeError(w,tc.err)
  if w.Code!=tc.status{t.Errorf("error=%v got=%d want=%d",tc.err,w.Code,tc.status)}
 }
}

func TestSyncStatusDistinguishesDesiredFromApplied(t *testing.T){
 desired:=uuid.New()
 previous:=uuid.New()
 status:=SyncStatus{
  SubscriptionID:uuid.New(),
  DesiredPublicIPID:&desired,
  AppliedPublicIPID:&previous,
  Revision:2,
  AppliedRevision:1,
  Status:"pending",
 }
 encoded,err:=json.Marshal(status)
 if err!=nil{t.Fatal(err)}
 var body map[string]any
 if err:=json.Unmarshal(encoded,&body);err!=nil{t.Fatal(err)}
 if body["desiredPublicIpId"]!=desired.String() || body["appliedPublicIpId"]!=previous.String(){
  t.Fatalf("status must distinguish desired from applied: %s",encoded)
 }
 status.AppliedPublicIPID=nil
 encoded,err=json.Marshal(status)
 if err!=nil{t.Fatal(err)}
 if !strings.Contains(string(encoded),`"appliedPublicIpId":null`){
  t.Fatalf("unapplied IP must be null: %s",encoded)
 }
}

func TestSubscriptionSelectionNullPreferenceJSON(t *testing.T){
 selection:=SubscriptionSelection{SubscriptionID:uuid.New()}
 data,err:=json.Marshal(selection)
 if err!=nil{t.Fatal(err)}
 if !strings.Contains(string(data),`"publicIpId":null`) || !strings.Contains(string(data),`"ipAddress":null`){
  t.Fatalf("missing selection must return null fields: %s",data)
 }
}

func TestSyncStatusUnconfiguredJSON(t *testing.T){
 status:=SyncStatus{SubscriptionID:uuid.New(),Status:"unconfigured"}
 data,err:=json.Marshal(status)
 if err!=nil{t.Fatal(err)}
 for _,expected:=range []string{`"desiredPublicIpId":null`,`"appliedPublicIpId":null`,`"requiresReconciliation":false`,`"updatedAt":null`,`"status":"unconfigured"`} {
  if !strings.Contains(string(data),expected){t.Fatalf("missing %s: %s",expected,data)}
 }
}

func TestSyncStatusReconciliationFlagJSON(t *testing.T){
 status:=SyncStatus{
  SubscriptionID:uuid.New(),
  Status:"failed",
  Revision:3,
  RequiresReconciliation:true,
 }
 data,err:=json.Marshal(status)
 if err!=nil{t.Fatal(err)}
 if !strings.Contains(string(data),`"requiresReconciliation":true`){
  t.Fatalf("expired synchronization must expose reconciliation flag: %s",data)
 }
 if strings.Contains(string(data),"lastError") || strings.Contains(string(data),"last_error"){
  t.Fatalf("internal error details must not be exposed: %s",data)
 }
}
