package publicip

import "testing"

func TestMatchesObserved(t *testing.T) {
 tests := []struct{
  name, saved, observed string
  want bool
 }{
  {"same public IPv4","8.8.8.8","8.8.8.8",true},
  {"different public IPv4","8.8.8.8","1.1.1.1",false},
  {"private observed","8.8.8.8","10.0.0.1",false},
  {"private saved","192.168.1.1","192.168.1.1",false},
  {"mapped IPv6","8.8.8.8","::ffff:8.8.8.8",false},
  {"malformed","8.8.8.8","invalid",false},
 }
 for _, tc := range tests {
  t.Run(tc.name,func(t *testing.T){
   if got:=MatchesObserved(tc.saved,tc.observed); got!=tc.want {
    t.Fatalf("MatchesObserved(%q,%q)=%v, want %v",tc.saved,tc.observed,got,tc.want)
   }
  })
 }
}
