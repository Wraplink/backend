package support

import (
 "encoding/json"
 "net/http"
 "net/mail"
 "strings"
)

func(h *Handler) PublicContact(w http.ResponseWriter,r *http.Request){
 var b struct{Name string `json:"name"`;Email string `json:"email"`;Subject string `json:"subject"`;Message string `json:"message"`;Website string `json:"website"`}
 decoder:=json.NewDecoder(http.MaxBytesReader(w,r.Body,16000));decoder.DisallowUnknownFields()
 if decoder.Decode(&b)!=nil{fail(w,400,"invalid contact request");return}
 b.Name=strings.TrimSpace(b.Name);b.Email=strings.TrimSpace(b.Email);b.Subject=strings.TrimSpace(b.Subject);b.Message=strings.TrimSpace(b.Message)
 if len(b.Name)<2||len(b.Name)>120||len(b.Email)>254||len(b.Subject)<3||len(b.Subject)>200||len(b.Message)<10||len(b.Message)>10000{fail(w,400,"invalid contact details");return}
 address,err:=mail.ParseAddress(b.Email)
 if err!=nil||address.Address!=b.Email{fail(w,400,"invalid email");return}
 // Honeypot field: accept without storage to avoid confirming bot detection.
 if b.Website!=""{respond(w,202,map[string]string{"status":"received"});return}
 _,err=h.DB.Exec(r.Context(),"INSERT INTO contact_messages(name,email,subject,message) VALUES($1,$2,$3,$4)",b.Name,b.Email,b.Subject,b.Message)
 if err!=nil{fail(w,500,"could not save message");return}
 respond(w,201,map[string]string{"status":"received"})
}

func(h *Handler) AdminContactMessages(w http.ResponseWriter,r *http.Request){
 if _,ok:=h.admin(w,r);!ok{return}
 rows,err:=h.DB.Query(r.Context(),"SELECT id,name,email,subject,message,created_at FROM contact_messages ORDER BY created_at DESC LIMIT 100")
 if err!=nil{fail(w,500,"could not load contact messages");return}
 defer rows.Close()
 items:=make([]map[string]any,0)
 for rows.Next(){var id,name,email,subject,message string;var created any
  if rows.Scan(&id,&name,&email,&subject,&message,&created)!=nil{fail(w,500,"could not read messages");return}
  items=append(items,map[string]any{"id":id,"name":name,"email":email,"subject":subject,"message":message,"createdAt":created})
 }
 if rows.Err()!=nil{fail(w,500,"could not read messages");return}
 respond(w,200,map[string]any{"messages":items})
}
