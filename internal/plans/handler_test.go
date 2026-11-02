package plans

import (
 "testing"
 "time"
)

func validInput() AdminPlanInput {
 return AdminPlanInput{Code:"starter",Name:"Starter",Currency:"IRR",DurationDays:30,PriceMinor:1000,DataLimitBytes:1024,Active:true}
}

func TestValidateAdminPlan(t *testing.T) {
 future:=time.Now().Add(24*time.Hour)
 cases:=[]struct{name string;change func(*AdminPlanInput);valid bool}{
  {"valid",func(*AdminPlanInput){},true},
  {"normalizes",func(p *AdminPlanInput){p.Code=" STARTER ";p.Currency="irr"},true},
  {"scheduled",func(p *AdminPlanInput){p.PublishAt=&future},true},
  {"invalid code",func(p *AdminPlanInput){p.Code="bad code"},false},
  {"empty name",func(p *AdminPlanInput){p.Name=" "},false},
  {"negative price",func(p *AdminPlanInput){p.PriceMinor=-1},false},
  {"zero duration",func(p *AdminPlanInput){p.DurationDays=0},false},
  {"negative data limit",func(p *AdminPlanInput){p.DataLimitBytes=-1},false},
  {"negative sort",func(p *AdminPlanInput){p.SortOrder=-1},false},
  {"invalid currency",func(p *AdminPlanInput){p.Currency="1RR"},false},
 }
 for _,tc:=range cases{t.Run(tc.name,func(t *testing.T){
  p:=validInput();tc.change(&p);err:=validateAdminPlan(&p)
  if (err==nil)!=tc.valid{t.Fatalf("valid=%v, err=%v",tc.valid,err)}
  if tc.name=="normalizes" && (p.Code!="starter"||p.Currency!="IRR"){t.Fatalf("normalization failed: %+v",p)}
 })}
}
