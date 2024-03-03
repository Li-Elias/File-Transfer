package models

import (
	"database/sql"
	"errors"
)

var (
	ErrRecordNotFound = errors.New("record not found")
	ErrEditConflict   = errors.New("edit conflict")
)

type Models struct {
	Users  UserModel
	Tokens TokenModel
	Files  FileModel
}

func NewModels(db *sql.DB) Models {
	return Models{
		Users:  UserModel{DB: db},
		Tokens: TokenModel{DB: db},
		Files:  FileModel{DB: db},
	}
}
