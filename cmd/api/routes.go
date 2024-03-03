package main

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/go-chi/httprate"
)

func (app *application) routes() http.Handler {
	router := chi.NewRouter()

	router.Use(app.Logger)
	router.Use(middleware.Recoverer)
	router.Use(cors.Handler(cors.Options{
		AllowedOrigins:   app.config.cors.allowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: false,
		MaxAge:           300,
	}))
	router.Use(app.authenticate)
	router.Use(httprate.Limit(
		10,
		1*time.Minute,
		httprate.WithLimitHandler(func(w http.ResponseWriter, r *http.Request) {
			app.tooManyRequests(w, r)
		}),
	))

	router.NotFound(app.notFoundResponse)
	router.MethodNotAllowed(app.methodNotAllowedResponse)

	router.Get("/healthcheck", app.healthcheckHandler)

	router.Group(func(router chi.Router) {
		router.Use(app.requireActivatedUser)
		router.Use(httprate.Limit(
			5,
			1*time.Minute,
			httprate.WithLimitHandler(func(w http.ResponseWriter, r *http.Request) {
				app.tooManyRequests(w, r)
			}),
		))

		router.Get("/users/files", app.listUserFilesHandler)
		router.Post("/users/files", app.uploadFileHandler)
		router.Get("/users/files/{id}", app.getUserFileHandler)
		router.Put("/users/files/{id}", app.updateUserFileHandler)
		router.Delete("/users/files/{id}", app.deleteUserFileHandler)
	})

	router.Get("/files/{code}", app.getFileFromCodeHandler)

	router.Post("/users", app.registerUserHandler)
	router.Put("/users/activated", app.activateUserHandler)
	router.Put("/users/password", app.updateUserPasswordHandler)

	router.Post("/tokens/authenticate", app.createAuthenticationTokenHandler)
	router.Post("/tokens/activation", app.createActivationTokenHandler)
	router.Post("/tokens/password-reset", app.createPasswordResetTokenHandler)

	return router
}
