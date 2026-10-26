package user

import (
 "context"
 "errors"
 "github.com/google/uuid"
 "github.com/jackc/pgx/v5"
)

func (r *Repository) GetAppearance(ctx context.Context,id uuid.UUID,theme *string) error {
 err:=r.db.QueryRow(ctx,"SELECT theme FROM user_appearance_preferences WHERE user_id=$1",id).Scan(theme)
 if errors.Is(err,pgx.ErrNoRows){*theme="midnight-black";return nil}
 return err
}
func (r *Repository) SetAppearance(ctx context.Context,id uuid.UUID,theme string) error {
 _,err:=r.db.Exec(ctx,"INSERT INTO user_appearance_preferences(user_id,theme) VALUES($1,$2) ON CONFLICT(user_id) DO UPDATE SET theme=EXCLUDED.theme,updated_at=now()",id,theme)
 return err
}
