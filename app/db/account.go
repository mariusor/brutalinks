package db

import "time"

type Account struct {
	Id        int64     `db:"id,auto"`
	Key       Key       `db:"key,size(64)"`
	Email     []byte    `db:"email"`
	Handle    string    `db:"handle"`
	Score     int64     `db:"score"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
	Flags     FlagBits  `db:"flags"`
	Metadata  Metadata  `db:"metadata"`
}
