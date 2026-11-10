package starterpolicy

import (
 "errors"
 "testing"
 "time"
)

func TestWindow(t *testing.T) {
 now:=time.Date(2026,11,9,12,0,0,0,time.UTC)
 w,err:=Window(now,now.Add(7*24*time.Hour),time.Hour,"reserved")
 if err!=nil {t.Fatal(err)}
 if !w.ExpiresAt.Equal(now.Add(time.Hour)) {t.Fatal("duration must begin at activation")}
 if _,err=Window(now,now,time.Hour,"reserved");!errors.Is(err,ErrReservationExpired) {t.Fatalf("expected expiry, got %v",err)}
 if _,err=Window(now,now.Add(time.Hour),time.Hour,"active");!errors.Is(err,ErrAlreadyActivated) {t.Fatalf("expected already activated, got %v",err)}
 if _,err=Window(now,now.Add(time.Hour),0,"reserved");!errors.Is(err,ErrInvalidDuration) {t.Fatalf("expected invalid duration, got %v",err)}
}

func TestWindowBoundaryAndClaimStates(t *testing.T) {
 now:=time.Date(2026,11,10,9,0,0,0,time.UTC)
 cases:=[]struct{
  name string
  deadline time.Time
  duration time.Duration
  state string
  want error
 }{
  {"just before reservation expiry",now.Add(time.Nanosecond),time.Second,"reserved",nil},
  {"exact reservation expiry",now,time.Second,"reserved",ErrReservationExpired},
  {"past reservation expiry",now.Add(-time.Nanosecond),time.Second,"reserved",ErrReservationExpired},
  {"missing reservation expiry",time.Time{},time.Second,"reserved",ErrReservationExpired},
  {"active claim cannot restart",now.Add(time.Hour),time.Hour,"active",ErrAlreadyActivated},
  {"cancelled claim cannot restart",now.Add(time.Hour),time.Hour,"cancelled",ErrAlreadyActivated},
  {"expired claim cannot restart",now.Add(time.Hour),time.Hour,"expired",ErrAlreadyActivated},
  {"zero duration",now.Add(time.Hour),0,"reserved",ErrInvalidDuration},
  {"negative duration",now.Add(time.Hour),-time.Second,"reserved",ErrInvalidDuration},
 }
 for _,tc:=range cases{
  t.Run(tc.name,func(t *testing.T){
   window,err:=Window(now,tc.deadline,tc.duration,tc.state)
   if !errors.Is(err,tc.want){t.Fatalf("Window() error = %v, want %v",err,tc.want)}
   if tc.want==nil{
    if !window.StartsAt.Equal(now)||!window.ExpiresAt.Equal(now.Add(tc.duration)){
     t.Fatalf("unexpected activation window: %+v",window)
    }
   }
  })
 }
}
