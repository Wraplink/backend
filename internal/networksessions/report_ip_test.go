package networksessions

import "testing"

func TestValidReportedClientIP(t *testing.T) {
 tests:=[]struct{address string;valid bool}{
  {"192.0.2.10",true},
  {"2001:db8::1",true},
  {"",false},
  {"192.0.2.10/32",false},
  {"192.0.2.10:443",false},
  {"::ffff:192.0.2.10",false},
  {"fe80::1%eth0",false},
  {"2001:0db8:0:0:0:0:0:1",false},
  {"192.0.2.010",false},
  {" 192.0.2.10",false},
 }
 for _,tc:=range tests {
  t.Run(tc.address,func(t *testing.T){
   if got:=validReportedClientIP(tc.address);got!=tc.valid {
    t.Fatalf("validReportedClientIP(%q)=%v, want %v",tc.address,got,tc.valid)
   }
  })
 }
}
