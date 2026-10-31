package email

import "testing"

func TestNewSMTPSenderValidation(t *testing.T) {
 base:=SMTPConfig{Host:"smtp.gmail.com",Port:587,From:"no-reply@example.com",FrontendURL:"https://example.com"}
 if _,err:=NewSMTPSender(base);err!=nil {t.Fatalf("valid SMTP configuration rejected: %v",err)}
 cases:=[]struct{name string;edit func(*SMTPConfig)}{
  {"missing host",func(c *SMTPConfig){c.Host=""}},
  {"invalid port",func(c *SMTPConfig){c.Port=0}},
  {"invalid sender",func(c *SMTPConfig){c.From="not-an-email"}},
  {"missing frontend",func(c *SMTPConfig){c.FrontendURL=""}},
  {"username without password",func(c *SMTPConfig){c.Username="user@example.com"}},
 }
 for _,tc:=range cases {t.Run(tc.name,func(t *testing.T){
  c:=base;tc.edit(&c)
  if _,err:=NewSMTPSender(c);err==nil {t.Fatal("expected SMTP validation error")}
 })}
}
