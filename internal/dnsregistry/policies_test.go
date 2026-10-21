package dnsregistry

import "testing"

func TestDomainPolicyValidation(t *testing.T) {
 tests:=[]struct{name string;policy DomainPolicy;valid bool}{
  {"normal",DomainPolicy{Domain:"example.com",Mode:"normal"},true},
  {"hijack",DomainPolicy{Domain:"sni.example.com",Mode:"hijack"},true},
  {"specific IPv4",DomainPolicy{Domain:"ip.example.com",Mode:"specific_ip",SpecificIP:"192.0.2.44"},true},
  {"specific IPv6",DomainPolicy{Domain:"ip.example.com",Mode:"specific_ip",SpecificIP:"2001:db8::44"},true},
  {"route",DomainPolicy{Domain:"route.example.com",Mode:"route",Route:&RoutePolicy{DestinationIP:"192.0.2.50",Protocol:"tcp"}},true},
  {"invalid domain",DomainPolicy{Domain:"bad/name",Mode:"normal"},false},
  {"invalid address",DomainPolicy{Domain:"ip.example.com",Mode:"specific_ip",SpecificIP:"bad"},false},
  {"missing route",DomainPolicy{Domain:"route.example.com",Mode:"route"},false},
  {"conflicting IP",DomainPolicy{Domain:"route.example.com",Mode:"route",SpecificIP:"192.0.2.2",Route:&RoutePolicy{DestinationIP:"192.0.2.3",Protocol:"tcp"}},false},
  {"invalid protocol",DomainPolicy{Domain:"route.example.com",Mode:"route",Route:&RoutePolicy{DestinationIP:"192.0.2.3",Protocol:"icmp"}},false},
 }
 for _,tc:=range tests{t.Run(tc.name,func(t *testing.T){if got:=validPolicy(tc.policy);got!=tc.valid{t.Fatalf("validPolicy=%v want %v",got,tc.valid)}})}
}
