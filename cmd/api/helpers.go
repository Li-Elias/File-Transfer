package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type envelope map[string]interface{}

var (
	letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890")
)

func (app *application) writeJSON(w http.ResponseWriter, status int, data envelope, headers http.Header) error {
	js, err := json.Marshal(data)
	if err != nil {
		return err
	}

	js = append(js, '\n')

	for key, value := range headers {
		w.Header()[key] = value
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(js)
	return nil
}

func (app *application) readJSON(w http.ResponseWriter, r *http.Request, dst interface{}) error {
	maxBytes := 1_048_576
	r.Body = http.MaxBytesReader(w, r.Body, int64(maxBytes))

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	err := dec.Decode(dst)
	if err != nil {
		var syntaxError *json.SyntaxError
		var unmarshalTypeError *json.UnmarshalTypeError
		var invalidUnmarshalError *json.InvalidUnmarshalError

		switch {
		case errors.As(err, &syntaxError):
			return fmt.Errorf("body contains badly-formed JSON (at character %d)", syntaxError.Offset)

		case errors.Is(err, io.ErrUnexpectedEOF):
			return errors.New("body contains badly-formed JSON")

		case errors.As(err, &unmarshalTypeError):
			if unmarshalTypeError.Field != "" {
				return fmt.Errorf("body contains incorrect JSON type for field %q", unmarshalTypeError.Field)
			}
			return fmt.Errorf("body contains incorrect JSON type (at character %d)", unmarshalTypeError.Offset)

		case errors.Is(err, io.EOF):
			return errors.New("body must not be empty")

		case strings.HasPrefix(err.Error(), "json: unknown field "):
			fieldName := strings.TrimPrefix(err.Error(), "json: unknown field ")
			return fmt.Errorf("body contains unknown key %s", fieldName)

		case err.Error() == "http: request body too large":
			return fmt.Errorf("body must not be larger than %d bytes", maxBytes)

		case errors.As(err, &invalidUnmarshalError):
			panic(err)

		default:
			return err
		}
	}

	err = dec.Decode(&struct{}{})
	if err != io.EOF {
		return errors.New("body must only contain a single JSON value")
	}

	return nil
}

func (app *application) background(fn func()) {
	app.waitgroup.Add(1)

	go func() {
		defer func() {
			defer app.waitgroup.Done()

			if err := recover(); err != nil {
				app.logger.PrintError(fmt.Errorf("%s", err), nil)
			}
		}()

		fn()
	}()
}

func (app *application) isFolderEmpty(dirPath string) (bool, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return false, err
	}

	return len(entries) == 0, nil
}

func (app *application) deleteEmptyFolder(dirPath string) error {
	isEmpty, err := app.isFolderEmpty(dirPath)
	if err != nil {
		return err
	}

	if isEmpty {
		err := os.Remove(dirPath)
		if err != nil {
			return err
		}
	}

	return nil
}

func (app *application) createFile(file multipart.File, file_path string) error {
	folder_path := filepath.Dir(file_path)

	err := os.MkdirAll(folder_path, os.ModePerm)
	if err != nil {
		return err
	}

	f, err := os.Create(file_path)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, file)
	if err != nil {
		return err
	}

	return nil
}

// exceptions for manual deleting
func (app *application) deleteFileInBackground(file_path string, file_id int64) error {
	folder_path := filepath.Dir(file_path)

	err := app.models.Files.Delete(file_id)
	if err != nil && err.Error() != "record not found" {
		return err
	}
	err = os.Remove(file_path)
	if err != nil && err.Error() != fmt.Sprintf("remove %s: no such file or directory", file_path) {
		return err
	}
	err = app.deleteEmptyFolder(folder_path)
	if err != nil && err.Error() != fmt.Sprintf("open %s: no such file or directory", folder_path) {
		return err
	}

	return nil
}

func (app *application) generateUniqueString() string {
	source := rand.NewSource(time.Now().UnixNano())
	r := rand.New(source)

	b := make([]rune, 8)
	for i := range b {
		b[i] = letterRunes[r.Intn(len(letterRunes))]
	}
	return string(b)
}
