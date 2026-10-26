package payments

import (
 "encoding/json"
 "net/http"
 "github.com/go-chi/chi/v5"
)

func (h *Handler) ListPaymentMethods(w http.ResponseWriter,r *http.Request) {
 rows,err:=h.service.repository.db.Query(r.Context(),"SELECT method,enabled FROM payment_method_settings ORDER BY method")
 if err!=nil {writeJSONError(w,500,"could not load payment methods");return}
 defer rows.Close()
 methods:=map[string]bool{}
 for rows.Next(){var method string;var enabled bool;if err=rows.Scan(&method,&enabled);err!=nil{writeJSONError(w,500,"could not load payment methods");return};methods[method]=enabled}
 if rows.Err()!=nil{writeJSONError(w,500,"could not load payment methods");return}
 writeJSON(w,200,map[string]any{"methods":methods})
}

func (h *Handler) AdminSetPaymentMethod(w http.ResponseWriter,r *http.Request) {
 if _,ok:=h.reviewAdmin(w,r);!ok{return}
 method:=chi.URLParam(r,"method")
 if method!="card"&&method!="isp"&&method!="crypto"{writeJSONError(w,400,"invalid payment method");return}
 var body struct{Enabled *bool `json:"enabled"`}
 decoder:=json.NewDecoder(http.MaxBytesReader(w,r.Body,1024));decoder.DisallowUnknownFields()
 if decoder.Decode(&body)!=nil||body.Enabled==nil{writeJSONError(w,400,"enabled is required");return}
 // Non-card processors are not implemented yet; don't enable a method customers cannot complete.
 if method!="card"&&*body.Enabled{writeJSONError(w,409,"payment provider is not implemented");return}
 _,err:=h.service.repository.db.Exec(r.Context(),"UPDATE payment_method_settings SET enabled=$2,updated_at=now() WHERE method=$1",method,*body.Enabled)
 if err!=nil{writeJSONError(w,500,"could not update payment method");return}
 writeJSON(w,200,map[string]any{"method":method,"enabled":*body.Enabled})
}

func paymentCardEnabled(r *http.Request,h *Handler) bool {
 var enabled bool
 return h.service.repository.db.QueryRow(r.Context(),"SELECT enabled FROM payment_method_settings WHERE method='card'").Scan(&enabled)==nil&&enabled
}
