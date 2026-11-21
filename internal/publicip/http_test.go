package publicip

import (
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
