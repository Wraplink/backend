package user

import (
 "backend/internal/requestcontext"
 "encoding/json"
 "errors"
 "io"
 "net/http"
 "github.com/jackc/pgx/v5"
)

var allowedThemes=map[string]bool{"midnight-black":true,"cyber-blue":true,"neon-purple":true,"toxic-green":true,"crimson-gaming":true,"golden-elite":true,"arctic-frost":true,"synthwave":true,"inferno":true,"titanium":true}
type AppearanceResponse struct { Theme string `json:"theme"` }

func (h *MeHandler) GetAppearance(w http.ResponseWriter,r *http.Request){
 id,ok:=requestcontext.UserID(r.Context());if !ok {http.Error(w,"authentication required",401);return}
 var theme string
 err:=h.service.GetAppearance(r.Context(),id,&theme)
 if err!=nil {http.Error(w,"failed to load appearance",500);return}
 w.Header().Set("Content-Type","application/json");_ = json.NewEncoder(w).Encode(AppearanceResponse{Theme:theme})
}
func (h *MeHandler) SetAppearance(w http.ResponseWriter,r *http.Request){
 id,ok:=requestcontext.UserID(r.Context());if !ok {http.Error(w,"authentication required",401);return}
 var req AppearanceResponse
 dec:=json.NewDecoder(http.MaxBytesReader(w,r.Body,2048));dec.DisallowUnknownFields()
 if err:=dec.Decode(&req);err!=nil {http.Error(w,"invalid request",400);return}
 var extra any;if err:=dec.Decode(&extra);!errors.Is(err,io.EOF){http.Error(w,"invalid request",400);return}
 if !allowedThemes[req.Theme] {http.Error(w,"invalid theme",400);return}
 if err:=h.service.SetAppearance(r.Context(),id,req.Theme);err!=nil {http.Error(w,"failed to save appearance",500);return}
 w.Header().Set("Content-Type","application/json");_ = json.NewEncoder(w).Encode(req)
}
var _ = pgx.ErrNoRows
