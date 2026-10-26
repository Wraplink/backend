package support

import (
 "encoding/json"
 "net/http"
 "net/mail"
 "strconv"
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
 q:=strings.TrimSpace(r.URL.Query().Get("search"))
 if len(q)>200{fail(w,400,"search too long");return}
 page:=1;limit:=20
 if v:=r.URL.Query().Get("page");v!=""{n,e:=strconv.Atoi(v);if e!=nil||n<1||n>1000000{fail(w,400,"invalid page");return};page=n}
 if v:=r.URL.Query().Get("limit");v!=""{n,e:=strconv.Atoi(v);if e!=nil||n<1||n>100{fail(w,400,"invalid limit");return};limit=n}
 var total int64
 filter:="WHERE ($1::text='' OR name ILIKE '%'||$1||'%' OR email ILIKE '%'||$1||'%' OR subject ILIKE '%'||$1||'%' OR message ILIKE '%'||$1||'%')"
 if err:=h.DB.QueryRow(r.Context(),"SELECT count(*) FROM contact_messages "+filter,q).Scan(&total);err!=nil{fail(w,500,"could not count contact messages");return}
 rows,err:=h.DB.Query(r.Context(),"SELECT id,name,email,subject,message,created_at FROM contact_messages "+filter+" ORDER BY created_at DESC,id DESC LIMIT $2 OFFSET $3",q,limit,(page-1)*limit)
 if err!=nil{fail(w,500,"could not load contact messages");return}
 defer rows.Close()
 items:=make([]map[string]any,0)
 for rows.Next(){var id,name,email,subject,message string;var created any
  if rows.Scan(&id,&name,&email,&subject,&message,&created)!=nil{fail(w,500,"could not read messages");return}
  items=append(items,map[string]any{"id":id,"name":name,"email":email,"subject":subject,"message":message,"createdAt":created})
 }
 if rows.Err()!=nil{fail(w,500,"could not read messages");return}
 respond(w,200,map[string]any{"messages":items,"total":total,"page":page,"limit":limit})
}
