package payments

import (
 "math"
 "testing"
)

func TestPercentageDiscount(t *testing.T) {
 cases:=[]struct{name string;amount,basisPoints,want int64}{
  {"quarter",10000,2500,2500},
  {"rounds down",99,3333,32},
  {"full discount",10000,10000,10000},
  {"large amount",math.MaxInt64,10000,math.MaxInt64},
  {"large half",math.MaxInt64,5000,math.MaxInt64/2},
  {"zero",0,10000,0},
  {"negative",100,-1,0},
 }
 for _,tc:=range cases {t.Run(tc.name,func(t *testing.T){
  got:=percentageDiscount(tc.amount,tc.basisPoints)
  if got!=tc.want {t.Fatalf("percentageDiscount(%d,%d)=%d; want %d",tc.amount,tc.basisPoints,got,tc.want)}
 })}
}
