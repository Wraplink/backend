package payments

import (
 "encoding/json"
 "net/http"
 "strings"
 "github.com/go-chi/chi/v5"
 "github.com/google/uuid"
)

type paymentCard struct {
 ID uuid.UUID `json:"id"`
 CardNumber string `json:"cardNumber"`
 CardholderName string `json:"cardholderName"`
 BankName string `json:"bankName"`
 Active bool `json:"active"`
}

func (h *Handler) ListPaymentCards(w http.ResponseWriter,r *http.Request) {
 rows,err:=h.service.repository.db.Query(r.Context(),"SELECT id,card_number,cardholder_name,bank_name,active FROM payment_cards WHERE active=true ORDER BY created_at,id")
 if err!=nil {writeJSONError(w,500,"could not load payment cards");return};defer rows.Close()
 cards:=make([]paymentCard,0)
 for rows.Next(){var card paymentCard;if err=rows.Scan(&card.ID,&card.CardNumber,&card.CardholderName,&card.BankName,&card.Active);err!=nil {writeJSONError(w,500,"could not load cards");return};cards=append(cards,card)}
 if rows.Err()!=nil {writeJSONError(w,500,"could not load cards");return}
 // A database sequence rotates assignments across all backend instances.
 // nextval is atomic; active cards are ordered deterministically.
 var selectedCardID *uuid.UUID
 if len(cards)>0 {
  var rotation int64
  if err:=h.service.repository.db.QueryRow(r.Context(),"SELECT nextval('payment_card_round_robin_seq')").Scan(&rotation);err!=nil {writeJSONError(w,500,"could not select payment card");return}
  id:=cards[(rotation-1)%int64(len(cards))].ID
  selectedCardID=&id
 }
 writeJSON(w,200,map[string]any{"cards":cards,"selectedCardId":selectedCardID})
}

func (h *Handler) AdminListPaymentCards(w http.ResponseWriter,r *http.Request) {
 if _,ok:=h.reviewAdmin(w,r);!ok{return}
 rows,err:=h.service.repository.db.Query(r.Context(),"SELECT id,card_number,cardholder_name,bank_name,active FROM payment_cards ORDER BY created_at DESC")
 if err!=nil {writeJSONError(w,500,"could not load cards");return};defer rows.Close()
 cards:=make([]paymentCard,0)
 for rows.Next(){var card paymentCard;if err=rows.Scan(&card.ID,&card.CardNumber,&card.CardholderName,&card.BankName,&card.Active);err!=nil {writeJSONError(w,500,"could not load cards");return};cards=append(cards,card)}
 if rows.Err()!=nil {writeJSONError(w,500,"could not load cards");return}
 writeJSON(w,200,map[string]any{"cards":cards})
}

func (h *Handler) AdminCreatePaymentCard(w http.ResponseWriter,r *http.Request) {
 if _,ok:=h.reviewAdmin(w,r);!ok{return}
 var body struct{CardNumber string `json:"cardNumber"`;CardholderName string `json:"cardholderName"`;BankName string `json:"bankName"`}
 d:=json.NewDecoder(http.MaxBytesReader(w,r.Body,4096));d.DisallowUnknownFields()
 if d.Decode(&body)!=nil {writeJSONError(w,400,"invalid card");return}
 body.CardNumber=strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(body.CardNumber)," ",""),"-","")
 body.CardholderName=strings.TrimSpace(body.CardholderName);body.BankName=strings.TrimSpace(body.BankName)
 if len(body.CardNumber)!=16 || len(body.CardholderName)==0 || len(body.BankName)==0 || len(body.CardholderName)>150 || len(body.BankName)>120 {writeJSONError(w,400,"invalid card details");return}
 for _,c:=range body.CardNumber {if c<'0'||c>'9'{writeJSONError(w,400,"card number must contain 16 digits");return}}
 var card paymentCard
 err:=h.service.repository.db.QueryRow(r.Context(),"INSERT INTO payment_cards(card_number,cardholder_name,bank_name) VALUES($1,$2,$3) RETURNING id,card_number,cardholder_name,bank_name,active",body.CardNumber,body.CardholderName,body.BankName).Scan(&card.ID,&card.CardNumber,&card.CardholderName,&card.BankName,&card.Active)
 if err!=nil {writeJSONError(w,409,"card could not be created or already exists");return}
 writeJSON(w,201,card)
}

func (h *Handler) AdminSetPaymentCardActive(w http.ResponseWriter,r *http.Request) {
 if _,ok:=h.reviewAdmin(w,r);!ok{return}
 id,err:=uuid.Parse(chi.URLParam(r,"cardID"));if err!=nil{writeJSONError(w,400,"invalid card ID");return}
 var body struct{Active bool `json:"active"`}
 d:=json.NewDecoder(http.MaxBytesReader(w,r.Body,1024));d.DisallowUnknownFields()
 if d.Decode(&body)!=nil{writeJSONError(w,400,"invalid request");return}
 tag,err:=h.service.repository.db.Exec(r.Context(),"UPDATE payment_cards SET active=$2,updated_at=now() WHERE id=$1",id,body.Active)
 if err!=nil{writeJSONError(w,500,"could not update card");return}
 if tag.RowsAffected()==0{writeJSONError(w,404,"card not found");return}
 writeJSON(w,200,map[string]any{"id":id,"active":body.Active})
}
