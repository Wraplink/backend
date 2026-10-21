package dnsdomains

import "testing"

func TestDomainInputValidate(t *testing.T) {
 tests:=[]struct{name string; input DomainInput; valid bool}{
  {"sni",DomainInput{Domain:"example.com",Mode:SNIHijack},true},
  {"direct",DomainInput{Domain:"api.example.com",Mode:Direct},true},
  {"empty",DomainInput{Domain:"",Mode:Direct},false},
  {"bad label",DomainInput{Domain:"bad_domain.com",Mode:Direct},false},
  {"bad mode",DomainInput{Domain:"example.com",Mode:"UNKNOWN"},false},
  {"negative priority",DomainInput{Domain:"example.com",Mode:FullRoute,Priority:-1},false},
 }
 for _,tt:=range tests {t.Run(tt.name,func(t *testing.T){errs:=tt.input.Validate();if (len(errs)==0)!=tt.valid {t.Fatalf("validation errors: %v",errs)}})}
}
