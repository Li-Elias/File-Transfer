package models

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/Li-Elias/File-Transfer/internal/validator"
)

var (
	ErrDuplicatePath = errors.New("duplicate path")
)

type File struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Size        int64     `json:"size"`
	Path        string    `json:"-"`
	Code        string    `json:"code"`
	Expiry      time.Time `json:"expiry"`
	CreatedAt   time.Time `json:"created_at"`
	LastUpdated time.Time `json:"last_updated"`
	UserID      int64     `json:"-"`
}

type FileModel struct {
	DB *sql.DB
}

func ValidateFile(v *validator.Validator, file *File) {
	v.Check(len(file.Name) <= 50, "file_name", "must not be more than 50 bytes long")
	v.Check(file.Size <= 1_000_000, "file_size", "must not be more than 1_000_000 bytes big")
	v.Check(len(file.Code) == 8, "code", "must be 8 bytes long")
}

func (m FileModel) Insert(file *File) error {
	query := `
		INSERT INTO files (name, size, path, code, expiry, user_id)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at, last_updated`

	args := []interface{}{file.Name, file.Size, file.Path, file.Code, file.Expiry, file.UserID}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := m.DB.QueryRowContext(ctx, query, args...).Scan(&file.ID, &file.CreatedAt, &file.LastUpdated)
	if err != nil {
		switch {
		case err.Error() == `pq: duplicate key value violates unique constraint "files_path_key"`:
			return ErrDuplicatePath
		default:
			return err
		}
	}

	return nil
}

func (m FileModel) GetFromUser(id int64, u *User) (*File, error) {
	if id < 1 {
		return nil, ErrRecordNotFound
	}

	query := `
		SELECT id, name, size, path, code, expiry, created_at, last_updated
		FROM files
		WHERE id = $1 AND user_id = $2 AND expiry > $3`

	args := []interface{}{id, u.ID, time.Now()}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var file File

	err := m.DB.QueryRowContext(ctx, query, args...).Scan(
		&file.ID,
		&file.Name,
		&file.Size,
		&file.Path,
		&file.Code,
		&file.Expiry,
		&file.CreatedAt,
		&file.LastUpdated,
	)

	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return nil, ErrRecordNotFound
		default:
			return nil, err
		}
	}

	return &file, nil
}

func (m FileModel) GetAllFromUser(u *User) ([]*File, error) {
	query := `
		SELECT id, name, size, path, code, expiry, created_at, last_updated
		FROM files
		WHERE user_id = $1 AND expiry > $2`

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	rows, err := m.DB.QueryContext(ctx, query, u.ID, time.Now())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	files := []*File{}

	for rows.Next() {
		var file File
		err := rows.Scan(
			&file.ID,
			&file.Name,
			&file.Size,
			&file.Path,
			&file.Code,
			&file.Expiry,
			&file.CreatedAt,
			&file.LastUpdated,
		)
		if err != nil {
			return nil, err
		}
		files = append(files, &file)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}

	return files, nil
}

func (m FileModel) GetFromCode(code string) (*File, error) {
	query := `
			SELECT id, name, size, path, code, expiry, created_at, last_updated
			FROM files
			WHERE code = $1 AND expiry > $2`

	var file File

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := m.DB.QueryRowContext(ctx, query, code, time.Now()).Scan(
		&file.ID,
		&file.Name,
		&file.Size,
		&file.Path,
		&file.Code,
		&file.Expiry,
		&file.CreatedAt,
		&file.LastUpdated,
	)

	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return nil, ErrRecordNotFound
		default:
			return nil, err
		}
	}

	return &file, nil
}

func (m FileModel) UpdateFromUser(path string, id int64, u *User, code string) (*File, error) {
	query := `
		UPDATE files
		SET expiry = $1, last_updated = $2, code = $3
		WHERE path = $4 AND id = $5 AND user_id = $6 AND expiry > $7
		RETURNING *`

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	args := []interface{}{
		time.Now().Add(2 * time.Minute),
		time.Now(),
		code,
		path,
		id,
		u.ID,
		time.Now(),
	}

	var file File

	err := m.DB.QueryRowContext(ctx, query, args...).Scan(
		&file.ID,
		&file.Name,
		&file.Size,
		&file.Path,
		&file.Code,
		&file.Expiry,
		&file.CreatedAt,
		&file.LastUpdated,
		&file.UserID,
	)
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return nil, ErrRecordNotFound
		default:
			return nil, err
		}
	}

	return &file, nil
}

func (m FileModel) Delete(id int64) error {
	if id < 1 {
		return ErrRecordNotFound
	}

	query := `
		DELETE FROM files
		WHERE id = $1 AND expiry > $2`

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	result, err := m.DB.ExecContext(ctx, query, id, time.Now())
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return ErrRecordNotFound
	}

	return nil
}

// also returns path
func (m FileModel) DeleteFromUser(id int64, u *User) (string, error) {
	if id < 1 {
		return "", ErrRecordNotFound
	}

	query := `
		DELETE FROM files
		WHERE id = $1 AND user_id = $2 AND expiry > $3
		RETURNING path`

	args := []interface{}{id, u.ID, time.Now()}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var path string

	err := m.DB.QueryRowContext(ctx, query, args...).Scan(&path)
	if err != nil {
		return "", err
	}

	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return "", ErrRecordNotFound
		default:
			return "", err
		}
	}

	return path, nil
}
