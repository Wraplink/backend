package bootstrap

import (
 "context"
 "log/slog"
 "testing"
)

func TestEnsureDefaultAdminDisabledWithoutCredentials(t *testing.T) {
 t.Setenv("WRAPLINK_BOOTSTRAP_ADMIN_EMAIL", "")
 t.Setenv("WRAPLINK_BOOTSTRAP_ADMIN_PASSWORD", "")
 if err := EnsureDefaultAdmin(context.Background(), nil, slog.Default()); err != nil {
  t.Fatalf("disabled bootstrap should not require database: %v", err)
 }
}

func TestEnsureDefaultAdminRejectsIncompleteCredentials(t *testing.T) {
 for _,tc:=range []struct{name,email,secret string}{
  {"missing password","admin@example.com",""},
  {"missing email","","a-strong-test-password"},
  {"short password","admin@example.com","short"},
  {"invalid email","not-an-email","a-strong-test-password"},
 } {
  t.Run(tc.name,func(t *testing.T){
   t.Setenv("WRAPLINK_BOOTSTRAP_ADMIN_EMAIL",tc.email)
   t.Setenv("WRAPLINK_BOOTSTRAP_ADMIN_PASSWORD",tc.secret)
   if err:=EnsureDefaultAdmin(context.Background(),nil,slog.Default());err==nil {
    t.Fatal("expected configuration validation error")
   }
  })
 }
}
