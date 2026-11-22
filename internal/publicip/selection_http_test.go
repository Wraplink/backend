package publicip

import (
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"

 "github.com/google/uuid"
)

func TestSelectionEndpointDisabledByDefault(t *testing.T){
 user:=uuid.New()
 subscription:=uuid.New()
 h:=Handler{AuthenticatedUserID:func(*http.Request)(uuid.UUID,bool){return user,true}}
 request:=httptest.NewRequest(http.MethodPut,"/subscriptions/"+subscription.String()+"/selection",strings.NewReader(`{"publicIpId":"`+uuid.NewString()+`"}`))
 response:=httptest.NewRecorder()
 h.Routes().ServeHTTP(response,request)
 if response.Code!=http.StatusServiceUnavailable{t.Fatalf("status=%d, want 503",response.Code)}
}

func TestSelectionEndpointRequiresAuthentication(t *testing.T){
 subscription:=uuid.New()
 h:=Handler{AuthenticatedUserID:func(*http.Request)(uuid.UUID,bool){return uuid.Nil,false}}
 request:=httptest.NewRequest(http.MethodPut,"/subscriptions/"+subscription.String()+"/selection",strings.NewReader("{}"))
 response:=httptest.NewRecorder()
 h.Routes().ServeHTTP(response,request)
 if response.Code!=http.StatusUnauthorized{t.Fatalf("status=%d, want 401",response.Code)}
}
