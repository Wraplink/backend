package payments

import (
 "testing"
 "time"
)

func TestPlanPayable(t *testing.T) {
 now:=time.Date(2026,11,2,12,0,0,0,time.UTC)
 before:=now.Add(-time.Second)
 after:=now.Add(time.Second)
 cases:=[]struct{name string;active bool;publish *time.Time;want bool}{
  {"enabled immediately",true,nil,true},
  {"disabled immediately",false,nil,false},
  {"enabled published",true,&before,true},
  {"enabled future",true,&after,false},
  {"disabled published",false,&before,false},
  {"disabled future",false,&after,false},
  {"publication boundary",true,&now,true},
 }
 for _,tc:=range cases {t.Run(tc.name,func(t *testing.T){
  if got:=planPayable(tc.active,tc.publish,now);got!=tc.want {t.Fatalf("got %v want %v",got,tc.want)}
 })}
}
