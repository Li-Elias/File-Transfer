package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/Li-Elias/File-Transfer/internal/models"
	"github.com/Li-Elias/File-Transfer/internal/validator"
	"github.com/go-chi/chi/v5"
)

func (app *application) uploadFileHandler(w http.ResponseWriter, r *http.Request) {
	file, handler, err := r.FormFile("file")
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}
	defer file.Close()

	user := app.contextGetUser(r)

	new_file := &models.File{
		Name:   handler.Filename,
		Size:   handler.Size,
		Path:   fmt.Sprintf("./cache/%s/%s", user.Email, handler.Filename),
		Code:   app.generateUniqueString(),
		Expiry: time.Now().Add(2 * time.Minute),
		UserID: user.ID,
	}

	v := validator.New()
	if models.ValidateFile(v, new_file); !v.Valid() {
		app.failedValidationResponse(w, r, v.Errors)
		return
	}

	err = app.models.Files.Insert(new_file)
	if err != nil {
		switch {
		case err.Error() == "duplicate path":
			v.AddError("file", "path already exists")
			app.failedValidationResponse(w, r, v.Errors)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	err = app.createFile(file, new_file.Path)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	// delete file after expiry or server shutdown
	timer := time.NewTimer(30 * time.Second)
	cancel := make(chan os.Signal, 1)
	signal.Notify(cancel, syscall.SIGINT, syscall.SIGTERM)

	app.background(func() {
		select {
		case <-timer.C:
			err := app.deleteFileInBackground(new_file.Path, new_file.ID)
			if err != nil {
				app.logger.PrintError(err, nil)
				return
			}
		case <-cancel:
			err := app.deleteFileInBackground(new_file.Path, new_file.ID)
			if err != nil {
				app.logger.PrintError(err, nil)
				return
			}
		}
	})

	err = app.writeJSON(w, http.StatusAccepted, envelope{"file": new_file}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

func (app *application) listUserFilesHandler(w http.ResponseWriter, r *http.Request) {
	user := app.contextGetUser(r)

	files, err := app.models.Files.GetAllFromUser(user)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	err = app.writeJSON(w, http.StatusOK, envelope{"files": files}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

func (app *application) getUserFileHandler(w http.ResponseWriter, r *http.Request) {
	id_str := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(id_str, 10, 64)
	if err != nil || id < 1 {
		app.notFoundResponse(w, r)
		return
	}

	user := app.contextGetUser(r)

	file, err := app.models.Files.GetFromUser(id, user)
	if err != nil {
		switch {
		case errors.Is(err, models.ErrRecordNotFound):
			app.notFoundResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	err = app.writeJSON(w, http.StatusOK, envelope{"file": file}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

func (app *application) updateUserFileHandler(w http.ResponseWriter, r *http.Request) {
	id_str := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(id_str, 10, 64)
	if err != nil || id < 1 {
		app.notFoundResponse(w, r)
		return
	}

	file, handler, err := r.FormFile("file")
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}
	defer file.Close()

	user := app.contextGetUser(r)

	file_path := fmt.Sprintf("./cache/%s/%s", user.Email, handler.Filename)

	updated_file, err := app.models.Files.UpdateFromUser(file_path, id, user, app.generateUniqueString())
	if err != nil {
		switch {
		case errors.Is(err, models.ErrRecordNotFound):
			app.notFoundResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	// check if path exists
	if _, err := os.Stat(file_path); err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	err = app.createFile(file, file_path)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	// delete file after expiry or server shutdown
	// exceptions for manual deleting
	timer := time.NewTimer(2 * time.Minute)
	cancel := make(chan os.Signal, 1)
	signal.Notify(cancel, syscall.SIGINT, syscall.SIGTERM)

	app.background(func() {
		select {
		case <-timer.C:
			err := app.deleteFileInBackground(file_path, id)
			if err != nil {
				app.logger.PrintError(err, nil)
				return
			}
		case <-cancel:
			err := app.deleteFileInBackground(file_path, id)
			if err != nil {
				app.logger.PrintError(err, nil)
				return
			}
		}
	})

	err = app.writeJSON(w, http.StatusAccepted, envelope{"file": updated_file}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

func (app *application) deleteUserFileHandler(w http.ResponseWriter, r *http.Request) {
	id_str := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(id_str, 10, 64)
	if err != nil || id < 1 {
		app.notFoundResponse(w, r)
		return
	}

	user := app.contextGetUser(r)

	path, err := app.models.Files.DeleteFromUser(id, user)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
	err = os.Remove(path)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}
	err = app.deleteEmptyFolder(filepath.Dir(path))
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}

	err = app.writeJSON(w, http.StatusOK, envelope{"message": "file successfully deleted"}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

func (app *application) getFileFromCodeHandler(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")

	file_data, err := app.models.Files.GetFromCode(code)
	if err != nil {
		switch {
		case errors.Is(err, models.ErrRecordNotFound):
			app.notFoundResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	file, err := os.Open(file_data.Path)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", file_data.Name))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", fileInfo.Size()))

	_, err = io.Copy(w, file)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}
}
