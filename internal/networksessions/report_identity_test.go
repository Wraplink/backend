package networksessions

import (
 "testing"
 "github.com/google/uuid"
)

func TestValidReportIdentity(t *testing.T) {
 user:=uuid.New()
 subscription:=uuid.New()
 tests:=[]struct{
  name,mode string
  user,subscription uuid.UUID
  allowed bool
 }{
  {"SNI without asserted identity","sni",uuid.Nil,uuid.Nil,true},
  {"SNI with asserted user","sni",user,uuid.Nil,false},
  {"SNI with asserted subscription","sni",uuid.Nil,subscription,false},
  {"SNI with both asserted","sni",user,subscription,false},
  {"route with both identifiers","route",user,subscription,true},
  {"route missing user","route",uuid.Nil,subscription,false},
  {"route missing subscription","route",user,uuid.Nil,false},
 }
 for _,tc:=range tests {
  t.Run(tc.name,func(t *testing.T){
   if got:=validReportIdentity(tc.mode,tc.user,tc.subscription);got!=tc.allowed {
    t.Fatalf("validReportIdentity(%q)=%t, want %t",tc.mode,got,tc.allowed)
   }
  })
 }
}
