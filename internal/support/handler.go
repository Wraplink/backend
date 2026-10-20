package support

import (
 "encoding/json"
 "net/http"
 "strings"
 "strconv"
 "time"
 "github.com/go-chi/chi/v5"
 "github.com/google/uuid"
 "github.com/jackc/pgx/v5"
 "github.com/jackc/pgx/v5/pgxpool"
 "backend/internal/requestcontext"
)
type Handler struct {DB *pgxpool.Pool}
func NewHandler(db *pgxpool.Pool)*Handler{return &Handler{DB:db}}
func respond(w http.ResponseWriter,status int,v any){w.Header().Set("Content-Type","application/json");w.WriteHeader(status);_ = json.NewEncoder(w).Encode(v)}
func fail(w http.ResponseWriter,status int,msg string){respond(w,status,map[string]string{"error":msg})}
func actor(w http.ResponseWriter,r *http.Request)(uuid.UUID,bool){id,ok:=requestcontext.UserID(r.Context());if !ok{fail(w,401,"authentication required")};return id,ok}
func (h *Handler) admin(w http.ResponseWriter,r *http.Request)(uuid.UUID,bool){
 id,ok:=actor(w,r);if !ok{return id,false}
 var allowed bool
 if h.DB.QueryRow(r.Context(),"SELECT EXISTS(SELECT 1 FROM access_admins WHERE user_id=$1 UNION SELECT 1 FROM payment_review_admins WHERE user_id=$1)",id).Scan(&allowed)!=nil{fail(w,500,"authorization unavailable");return id,false}
 if !allowed{fail(w,403,"support staff permission required")}
 return id,allowed
}
type ticket struct {
 ID uuid.UUID `json:"id"`
 TicketNumber int64 `json:"ticketNumber"`
 UserID uuid.UUID `json:"userId"`
 Email string `json:"email"`
 FullName string `json:"fullName"`
 OrderNumber *int64 `json:"orderNumber"`
 Subject string `json:"subject"`
 Category string `json:"category"`
 Priority string `json:"priority"`
 Status string `json:"status"`
 CreatedAt time.Time `json:"createdAt"`
 UpdatedAt time.Time `json:"updatedAt"`
}
func (h *Handler) list(w http.ResponseWriter,r *http.Request,staff bool){
 id,ok:=actor(w,r);if !ok{return}
 if staff {if _,ok=h.admin(w,r);!ok{return}}
 limit:=50;offset:=0
 if raw:=r.URL.Query().Get("offset");raw!=""{n,e:=strconv.Atoi(raw);if e!=nil||n<0||n>1000000{fail(w,400,"invalid offset");return};offset=n}
 if raw:=r.URL.Query().Get("limit");raw!=""{n,e:=strconv.Atoi(raw);if e!=nil||n<1||n>100{fail(w,400,"invalid limit");return};limit=n}
 search:=strings.TrimSpace(r.URL.Query().Get("search"))
 if len(search)>200{fail(w,400,"search too long");return}
 status:=r.URL.Query().Get("status")
 if status!=""&&status!="open"&&status!="pending"&&status!="resolved"&&status!="closed"{fail(w,400,"invalid status");return}
 rows,err:=h.DB.Query(r.Context(),`SELECT t.id,t.ticket_number,t.user_id,u.email,u.full_name,s.order_number,t.subject,t.category,t.priority,t.status,t.created_at,t.updated_at
 FROM support_tickets t JOIN users u ON u.id=t.user_id LEFT JOIN user_subscriptions s ON s.id=t.order_id
 WHERE ($1::boolean OR t.user_id=$2) AND ($3::text='' OR t.status=$3)
 AND ($4::text='' OR t.subject ILIKE '%'||$4||'%' OR u.email ILIKE '%'||$4||'%' OR u.full_name ILIKE '%'||$4||'%' OR t.ticket_number::text=$4 OR s.order_number::text=$4)
 ORDER BY t.updated_at DESC,t.id DESC LIMIT $5 OFFSET $6`,staff,id,status,search,limit,offset)
 if err!=nil{fail(w,500,"could not list tickets");return};defer rows.Close()
 result:=make([]ticket,0)
 for rows.Next(){var t ticket;if rows.Scan(&t.ID,&t.TicketNumber,&t.UserID,&t.Email,&t.FullName,&t.OrderNumber,&t.Subject,&t.Category,&t.Priority,&t.Status,&t.CreatedAt,&t.UpdatedAt)!=nil{fail(w,500,"could not read ticket");return};result=append(result,t)}
 if rows.Err()!=nil{fail(w,500,"could not read tickets");return}
 respond(w,200,map[string]any{"tickets":result,"limit":limit,"offset":offset})
}
func(h *Handler) MyList(w http.ResponseWriter,r *http.Request){h.list(w,r,false)}
func(h *Handler) AdminList(w http.ResponseWriter,r *http.Request){h.list(w,r,true)}
func(h *Handler) Create(w http.ResponseWriter,r *http.Request){
 id,ok:=actor(w,r);if !ok{return}
 var b struct{Subject string `json:"subject"`;Category string `json:"category"`;Priority string `json:"priority"`;Message string `json:"message"`;OrderNumber *int64 `json:"orderNumber"`}
 if json.NewDecoder(http.MaxBytesReader(w,r.Body,16000)).Decode(&b)!=nil{fail(w,400,"invalid request");return}
 b.Subject=strings.TrimSpace(b.Subject);b.Message=strings.TrimSpace(b.Message)
 if len(b.Subject)<3||len(b.Subject)>200||len(b.Message)<1||len(b.Message)>10000{fail(w,400,"subject or message length invalid");return}
 if b.Priority==""{b.Priority="medium"}
 if b.Priority!="low"&&b.Priority!="medium"&&b.Priority!="high"{fail(w,400,"invalid priority");return}
 if b.Category==""{b.Category="general"}
 if len(b.Category)>40{fail(w,400,"invalid category");return}
 tx,e:=h.DB.Begin(r.Context());if e!=nil{fail(w,500,"database unavailable");return};defer tx.Rollback(r.Context())
 var orderID *uuid.UUID
 if b.OrderNumber!=nil {var v uuid.UUID;e=tx.QueryRow(r.Context(),"SELECT id FROM user_subscriptions WHERE user_id=$1 AND order_number=$2",id,*b.OrderNumber).Scan(&v);if e!=nil{fail(w,400,"order not found");return};orderID=&v}
 var ticketID uuid.UUID;var number int64
 e=tx.QueryRow(r.Context(),`INSERT INTO support_tickets(user_id,order_id,subject,category,priority) VALUES($1,$2,$3,$4,$5) RETURNING id,ticket_number`,id,orderID,b.Subject,b.Category,b.Priority).Scan(&ticketID,&number)
 if e==nil{_,e=tx.Exec(r.Context(),"INSERT INTO support_messages(ticket_id,sender_id,sender_role,message) VALUES($1,$2,'user',$3)",ticketID,id,b.Message)}
 if e!=nil{fail(w,500,"could not create ticket");return}
 if tx.Commit(r.Context())!=nil{fail(w,500,"could not save ticket");return}
 respond(w,201,map[string]any{"id":ticketID,"ticketNumber":number})
}
func(h *Handler) detail(w http.ResponseWriter,r *http.Request,staff bool){
 actorID,ok:=actor(w,r);if !ok{return}
 if staff {if _,ok=h.admin(w,r);!ok{return}}
 id,e:=uuid.Parse(chi.URLParam(r,"ticketID"));if e!=nil{fail(w,400,"invalid ticket ID");return}
 var t ticket
 e=h.DB.QueryRow(r.Context(),`SELECT t.id,t.ticket_number,t.user_id,u.email,u.full_name,s.order_number,t.subject,t.category,t.priority,t.status,t.created_at,t.updated_at FROM support_tickets t JOIN users u ON u.id=t.user_id LEFT JOIN user_subscriptions s ON s.id=t.order_id WHERE t.id=$1 AND ($2::boolean OR t.user_id=$3)`,id,staff,actorID).Scan(&t.ID,&t.TicketNumber,&t.UserID,&t.Email,&t.FullName,&t.OrderNumber,&t.Subject,&t.Category,&t.Priority,&t.Status,&t.CreatedAt,&t.UpdatedAt)
 if e==pgx.ErrNoRows{fail(w,404,"ticket not found");return};if e!=nil{fail(w,500,"could not load ticket");return}
 rows,e:=h.DB.Query(r.Context(),"SELECT id,sender_id,sender_role,message,created_at FROM support_messages WHERE ticket_id=$1 ORDER BY created_at,id",id)
 if e!=nil{fail(w,500,"could not load messages");return};defer rows.Close()
 messages:=make([]map[string]any,0)
 for rows.Next(){var mid,sender uuid.UUID;var role,msg string;var created time.Time;if rows.Scan(&mid,&sender,&role,&msg,&created)!=nil{fail(w,500,"could not read messages");return};messages=append(messages,map[string]any{"id":mid,"senderId":sender,"sender":role,"message":msg,"date":created})}
 if rows.Err()!=nil{fail(w,500,"could not read messages");return}
 respond(w,200,map[string]any{"ticket":t,"messages":messages})
}
func(h *Handler) MyDetail(w http.ResponseWriter,r *http.Request){h.detail(w,r,false)}
func(h *Handler) AdminDetail(w http.ResponseWriter,r *http.Request){h.detail(w,r,true)}
func(h *Handler) message(w http.ResponseWriter,r *http.Request,staff bool){
 id,ok:=actor(w,r);if !ok{return};if staff{if _,ok=h.admin(w,r);!ok{return}}
 tid,e:=uuid.Parse(chi.URLParam(r,"ticketID"));if e!=nil{fail(w,400,"invalid ticket ID");return}
 var b struct{Message string `json:"message"`};if json.NewDecoder(http.MaxBytesReader(w,r.Body,12000)).Decode(&b)!=nil{fail(w,400,"invalid request");return};b.Message=strings.TrimSpace(b.Message)
 if len(b.Message)<1||len(b.Message)>10000{fail(w,400,"invalid message");return}
 tx,e:=h.DB.Begin(r.Context());if e!=nil{fail(w,500,"database unavailable");return};defer tx.Rollback(r.Context())
 var status string
 e=tx.QueryRow(r.Context(),"SELECT status FROM support_tickets WHERE id=$1 AND ($2::boolean OR user_id=$3) FOR UPDATE",tid,staff,id).Scan(&status)
 if e==pgx.ErrNoRows{fail(w,404,"ticket not found");return};if e!=nil{fail(w,500,"database error");return}
 if status=="closed"{fail(w,409,"closed ticket cannot receive messages");return}
 role:="user";next:="open";if staff{role="support";next="pending"}
 _,e=tx.Exec(r.Context(),"INSERT INTO support_messages(ticket_id,sender_id,sender_role,message) VALUES($1,$2,$3,$4)",tid,id,role,b.Message)
 if e==nil{_,e=tx.Exec(r.Context(),"UPDATE support_tickets SET status=$2,updated_at=now() WHERE id=$1",tid,next)}
 if e!=nil{fail(w,500,"could not send message");return}
 if tx.Commit(r.Context())!=nil{fail(w,500,"could not save message");return}
 respond(w,201,map[string]string{"status":next})
}
func(h *Handler) MyMessage(w http.ResponseWriter,r *http.Request){h.message(w,r,false)}
func(h *Handler) AdminMessage(w http.ResponseWriter,r *http.Request){h.message(w,r,true)}
func(h *Handler) updateStatus(w http.ResponseWriter,r *http.Request,staff bool){
 id,ok:=actor(w,r);if !ok{return};if staff{if _,ok=h.admin(w,r);!ok{return}}
 tid,e:=uuid.Parse(chi.URLParam(r,"ticketID"));if e!=nil{fail(w,400,"invalid ticket ID");return}
 var b struct{Status string `json:"status"`};if json.NewDecoder(http.MaxBytesReader(w,r.Body,1024)).Decode(&b)!=nil{fail(w,400,"invalid request");return}
 if staff {if b.Status!="open"&&b.Status!="pending"&&b.Status!="resolved"&&b.Status!="closed"{fail(w,400,"invalid status");return}} else if b.Status!="closed"&&b.Status!="open"{fail(w,403,"invalid customer status");return}
 tag,e:=h.DB.Exec(r.Context(),"UPDATE support_tickets SET status=$3,updated_at=now() WHERE id=$1 AND ($2::boolean OR user_id=$4)",tid,staff,b.Status,id)
 if e!=nil{fail(w,500,"could not update ticket");return};if tag.RowsAffected()==0{fail(w,404,"ticket not found");return}
 respond(w,200,map[string]string{"status":b.Status})
}
func(h *Handler) MyStatus(w http.ResponseWriter,r *http.Request){h.updateStatus(w,r,false)}
func(h *Handler) AdminStatus(w http.ResponseWriter,r *http.Request){h.updateStatus(w,r,true)}
