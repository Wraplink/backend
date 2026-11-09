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
