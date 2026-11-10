package starterpolicy

import (
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
)

// Malformed admin requests must fail before attempting any database write.
func TestAdminCreateRejectsInvalidRequests(t *testing.T) {
 cases:=[]struct{name,body string}{
  {"invalid JSON",`{"planId":`},
  {"unknown field",`{"unexpected":true}`},
  {"multiple JSON values",`{} {}`},
  {"zero reservation TTL",`{"reservationTtlSeconds":0,"trialDurationSeconds":3600}`},
  {"negative reservation TTL",`{"reservationTtlSeconds":-1,"trialDurationSeconds":3600}`},
  {"reservation TTL too long",`{"reservationTtlSeconds":31536001,"trialDurationSeconds":3600}`},
  {"zero trial duration",`{"reservationTtlSeconds":604800,"trialDurationSeconds":0}`},
  {"negative trial duration",`{"reservationTtlSeconds":604800,"trialDurationSeconds":-1}`},
  {"trial duration too long",`{"reservationTtlSeconds":604800,"trialDurationSeconds":31536001}`},
  {"overflowing JSON number",`{"reservationTtlSeconds":604800,"trialDurationSeconds":9223372036854775808}`},
 }
 h:=AdminHandler{}
 for _,tc:=range cases {
  t.Run(tc.name,func(t *testing.T){
   req:=httptest.NewRequest(http.MethodPost,"/api/v1/admin/starter-policies/",strings.NewReader(tc.body))
   rec:=httptest.NewRecorder()
   h.Create(rec,req)
   if rec.Code!=http.StatusBadRequest {
    t.Fatalf("status=%d body=%s; want 400",rec.Code,rec.Body.String())
   }
  })
 }
}
