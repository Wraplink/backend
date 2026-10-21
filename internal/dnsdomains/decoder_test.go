package dnsdomains

import (
 "net/http/httptest"
 "strings"
 "testing"
)

func TestDecodeInputNormalization(t *testing.T) {
 r:=httptest.NewRequest("POST","/",strings.NewReader(`{"domain":" EXAMPLE.COM. ","enabled":true,"mode":"DIRECT","priority":100}`))
 d,issues,err:=DecodeInput(r)
 if err!=nil||len(issues)!=0 {t.Fatalf("decode: %v, validation: %v",err,issues)}
 if d.Domain!="example.com" || d.FallbackMode!="DIRECT" {t.Fatalf("unexpected normalized input: %+v",d)}
}
func TestDecodeInputRejectsUnknownField(t *testing.T) {
 r:=httptest.NewRequest("POST","/",strings.NewReader(`{"domain":"example.com","mode":"DIRECT","unknown":true}`))
 _,_,err:=DecodeInput(r)
 if err==nil {t.Fatal("expected unknown field error")}
}
