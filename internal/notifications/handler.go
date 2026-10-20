package notifications
import (
 "encoding/json"
 "net/http"
 "strconv"
 "github.com/go-chi/chi/v5"
 "github.com/google/uuid"
 "github.com/jackc/pgx/v5/pgxpool"
 "backend/internal/requestcontext"
)
type Handler struct{DB *pgxpool.Pool}
func NewHandler(db *pgxpool.Pool)*Handler{return &Handler{DB:db}}
func write(w http.ResponseWriter,code int,v any){w.Header().Set("Content-Type","application/json");w.WriteHeader(code);_ = json.NewEncoder(w).Encode(v)}
func user(w http.ResponseWriter,r *http.Request)(uuid.UUID,bool){id,ok:=requestcontext.UserID(r.Context());if !ok{write(w,401,map[string]string{"error":"authentication required"})};return id,ok}
func(h *Handler) List(w http.ResponseWriter,r *http.Request){
 id,ok:=user(w,r);if !ok{return}
 limit:=50;offset:=0
 if v:=r.URL.Query().Get("limit");v!=""{n,e:=strconv.Atoi(v);if e!=nil||n<1||n>100{write(w,400,map[string]string{"error":"invalid limit"});return};limit=n}
 if v:=r.URL.Query().Get("offset");v!=""{n,e:=strconv.Atoi(v);if e!=nil||n<0||n>1000000{write(w,400,map[string]string{"error":"invalid offset"});return};offset=n}
 rows,e:=h.DB.Query(r.Context(),"SELECT id,type,title,message,link,created_at,read_at IS NOT NULL FROM user_notifications WHERE user_id=$1 ORDER BY created_at DESC,id DESC LIMIT $2 OFFSET $3",id,limit,offset)
 if e!=nil{write(w,500,map[string]string{"error":"could not list notifications"});return};defer rows.Close()
 items:=make([]map[string]any,0)
 for rows.Next(){var nid uuid.UUID;var typ,title,msg string;var link *string;var created any;var read bool
  if rows.Scan(&nid,&typ,&title,&msg,&link,&created,&read)!=nil{write(w,500,map[string]string{"error":"could not read notifications"});return}
  items=append(items,map[string]any{"id":nid,"type":typ,"title":title,"message":msg,"link":link,"createdAt":created,"read":read})
 }
 if rows.Err()!=nil{write(w,500,map[string]string{"error":"could not read notifications"});return}
 var unread int64
 if h.DB.QueryRow(r.Context(),"SELECT count(*) FROM user_notifications WHERE user_id=$1 AND read_at IS NULL",id).Scan(&unread)!=nil{write(w,500,map[string]string{"error":"could not count notifications"});return}
 write(w,200,map[string]any{"notifications":items,"unreadCount":unread,"limit":limit,"offset":offset})
}
func(h *Handler) Read(w http.ResponseWriter,r *http.Request){
 id,ok:=user(w,r);if !ok{return}
 nid,e:=uuid.Parse(chi.URLParam(r,"notificationID"));if e!=nil{write(w,400,map[string]string{"error":"invalid notification ID"});return}
 tag,e:=h.DB.Exec(r.Context(),"UPDATE user_notifications SET read_at=COALESCE(read_at,now()) WHERE id=$1 AND user_id=$2",nid,id)
 if e!=nil{write(w,500,map[string]string{"error":"could not mark read"});return};if tag.RowsAffected()==0{write(w,404,map[string]string{"error":"notification not found"});return}
 write(w,200,map[string]bool{"ok":true})
}
func(h *Handler) ReadAll(w http.ResponseWriter,r *http.Request){
 id,ok:=user(w,r);if !ok{return}
 if _,e:=h.DB.Exec(r.Context(),"UPDATE user_notifications SET read_at=now() WHERE user_id=$1 AND read_at IS NULL",id);e!=nil{write(w,500,map[string]string{"error":"could not mark all read"});return}
 write(w,200,map[string]bool{"ok":true})
}
type preferences struct{Orders bool `json:"orders"`;Payments bool `json:"payments"`;Support bool `json:"support"`;Security bool `json:"security"`;System bool `json:"system"`}
func(h *Handler) Preferences(w http.ResponseWriter,r *http.Request){
 id,ok:=user(w,r);if !ok{return}
 var p preferences
 e:=h.DB.QueryRow(r.Context(),`SELECT COALESCE((SELECT orders FROM notification_preferences WHERE user_id=$1),true),COALESCE((SELECT payments FROM notification_preferences WHERE user_id=$1),true),COALESCE((SELECT support FROM notification_preferences WHERE user_id=$1),true),COALESCE((SELECT security FROM notification_preferences WHERE user_id=$1),true),COALESCE((SELECT system FROM notification_preferences WHERE user_id=$1),true)`,id).Scan(&p.Orders,&p.Payments,&p.Support,&p.Security,&p.System)
 if e!=nil{write(w,500,map[string]string{"error":"could not load preferences"});return};write(w,200,p)
}
func(h *Handler) SavePreferences(w http.ResponseWriter,r *http.Request){
 id,ok:=user(w,r);if !ok{return}
 var p preferences
 d:=json.NewDecoder(http.MaxBytesReader(w,r.Body,2048));d.DisallowUnknownFields()
 if d.Decode(&p)!=nil{write(w,400,map[string]string{"error":"invalid preferences"});return}
 _,e:=h.DB.Exec(r.Context(),`INSERT INTO notification_preferences(user_id,orders,payments,support,security,system) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(user_id) DO UPDATE SET orders=excluded.orders,payments=excluded.payments,support=excluded.support,security=excluded.security,system=excluded.system`,id,p.Orders,p.Payments,p.Support,p.Security,p.System)
 if e!=nil{write(w,500,map[string]string{"error":"could not save preferences"});return};write(w,200,p)
}
