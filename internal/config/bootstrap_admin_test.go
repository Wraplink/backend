package config

import (
 "strings"
 "testing"
)

func TestBootstrapAdminValidation(t *testing.T) {
 valid := Config{
  App: AppConfig{Name:"wraplink", Environment:"development", FrontendURL:"http://localhost:3000"},
  HTTP: HTTPConfig{Host:"127.0.0.1",Port:"7080"},
  Database: DatabaseConfig{URL:"postgres://localhost/wraplink",MaxConns:2},
  Security: SecurityConfig{JWTSecret:strings.Repeat("x",64),AccessTokenTTL:1,RefreshTokenTTL:2},
  Network: NetworkConfig{ControlNodeOfflineAfter:1},
 }
 cases:=[]struct{name,email,password string;wantErr bool}{
  {"disabled","","",false},
  {"valid","admin@example.com","correct-horse-battery-staple",false},
  {"missing password","admin@example.com","",true},
  {"missing email","","correct-horse-battery-staple",true},
  {"weak password","admin@example.com","short",true},
  {"invalid email","not-an-email","correct-horse-battery-staple",true},
 }
 for _,tc:=range cases {
  t.Run(tc.name,func(t *testing.T){
   cfg:=valid
   cfg.Security.BootstrapAdminEmail=tc.email
   cfg.Security.BootstrapAdminPassword=tc.password
   err:=validate(cfg)
   if (err!=nil)!=tc.wantErr {t.Fatalf("validate() error = %v; want error %v",err,tc.wantErr)}
  })
 }
}
