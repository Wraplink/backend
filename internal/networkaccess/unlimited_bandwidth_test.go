package networkaccess

import (
 "testing"
 "time"
)

func TestAccessStatusUnlimitedBandwidth(t *testing.T) {
 now := time.Now()
 start := now.Add(-time.Hour)
 end := now.Add(time.Hour)
 cases := []struct {
  name string
  unlimited bool
  limit int64
  used int64
  want string
 }{
  {"unlimited zero limit",true,0,0,AccessStatusActive},
  {"unlimited with usage",true,0,100000,AccessStatusActive},
  {"limited zero quota",false,0,0,AccessStatusQuotaExhausted},
  {"limited under quota",false,100,99,AccessStatusActive},
  {"limited exhausted",false,100,100,AccessStatusQuotaExhausted},
 }
 for _,tc:=range cases {
  t.Run(tc.name,func(t *testing.T){
   state:=&subscriptionState{Status:"active",StartsAt:&start,ExpiresAt:&end,UnlimitedBandwidth:tc.unlimited,DataLimitBytes:tc.limit,UsedBytes:tc.used}
   if got:=accessStatus(state,now);got!=tc.want {t.Fatalf("accessStatus()=%q want %q",got,tc.want)}
  })
 }
}
