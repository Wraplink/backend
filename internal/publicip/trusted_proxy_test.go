package publicip

import (
 "net/http"
 "net/netip"
 "testing"
)

func TestTrustedClientIPv4(t *testing.T) {
 trusted := []netip.Prefix{netip.MustParsePrefix("172.20.0.0/16")}
 cases := []struct{
  name, peer, real, forwarded, want string
  ok bool
 }{
  {"trusted nginx", "172.20.0.5:1234", "8.8.8.8", "", "8.8.8.8", true},
  {"untrusted spoof", "1.1.1.1:1234", "8.8.8.8", "8.8.4.4", "1.1.1.1", true},
  {"trusted missing header", "172.20.0.5:1234", "", "", "", false},
  {"trusted private address", "172.20.0.5:1234", "192.168.1.2", "", "", false},
  {"trusted comma injection", "172.20.0.5:1234", "8.8.8.8,1.1.1.1", "", "", false},
  {"untrusted private peer", "10.0.0.5:1234", "8.8.8.8", "", "", false},
 }
 for _, tc := range cases {
  t.Run(tc.name,func(t *testing.T){
   r:=&http.Request{RemoteAddr:tc.peer,Header:make(http.Header)}
   if tc.real!="" {r.Header.Set("X-Real-IP",tc.real)}
   if tc.forwarded!="" {r.Header.Set("X-Forwarded-For",tc.forwarded)}
   got,err:=TrustedClientIPv4(r,trusted)
   if (err==nil)!=tc.ok || (tc.ok && got!=tc.want) {
    t.Fatalf("got %q, err=%v, want %q, ok=%v",got,err,tc.want,tc.ok)
   }
  })
 }
}

func TestTrustedClientIPv4RejectsDuplicateHeaders(t *testing.T) {
 r:=&http.Request{RemoteAddr:"172.20.0.5:1234",Header:make(http.Header)}
 r.Header.Add("X-Real-IP","8.8.8.8")
 r.Header.Add("X-Real-IP","1.1.1.1")
 _,err:=TrustedClientIPv4(r,[]netip.Prefix{netip.MustParsePrefix("172.20.0.0/16")})
 if err==nil {t.Fatal("expected duplicate header rejection")}
}
