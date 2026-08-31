package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/nself-org/nself-meetings/internal"
)

func main() {
	cfg := internal.LoadConfig()

	pool, err := internal.NewDB(context.Background(), cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer pool.Close()

	if err := internal.InitSchema(context.Background(), pool); err != nil {
		log.Fatalf("schema init: %v", err)
	}

	h := internal.NewHandlers(pool)

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	r.Get("/health", h.Health)
	r.Get("/ready", h.Ready)

	// Calendars
	r.Route("/api/v1/calendars", func(r chi.Router) {
		r.Get("/", h.ListCalendars)
		r.Post("/", h.CreateCalendar)
		r.Get("/{id}", h.GetCalendar)
		r.Put("/{id}", h.UpdateCalendar)
		r.Delete("/{id}", h.DeleteCalendar)
		r.Get("/{calendar_id}/shares", h.ListCalendarShares)
		r.Post("/{calendar_id}/shares", h.CreateCalendarShare)
		r.Delete("/{calendar_id}/shares/{id}", h.DeleteCalendarShare)
	})

	// Rooms
	r.Route("/api/v1/rooms", func(r chi.Router) {
		r.Get("/", h.ListRooms)
		r.Post("/", h.CreateRoom)
		r.Get("/{id}", h.GetRoom)
		r.Put("/{id}", h.UpdateRoom)
		r.Delete("/{id}", h.DeleteRoom)
	})

	// Events
	r.Route("/api/v1/events", func(r chi.Router) {
		r.Get("/", h.ListEvents)
		r.Post("/", h.CreateEvent)
		r.Get("/{id}", h.GetEvent)
		r.Put("/{id}", h.UpdateEvent)
		r.Delete("/{id}", h.DeleteEvent)
		r.Get("/{event_id}/attendees", h.ListAttendees)
		r.Post("/{event_id}/attendees", h.CreateAttendee)
		r.Put("/{event_id}/attendees/{id}/rsvp", h.UpdateAttendeeRSVP)
		r.Delete("/{event_id}/attendees/{id}", h.DeleteAttendee)
		r.Get("/{event_id}/reminders", h.ListReminders)
		r.Post("/{event_id}/reminders", h.CreateReminder)
		r.Delete("/{event_id}/reminders/{id}", h.DeleteReminder)
	})

	// External Calendars
	r.Route("/api/v1/external-calendars", func(r chi.Router) {
		r.Get("/", h.ListExternalCalendars)
		r.Post("/", h.CreateExternalCalendar)
		r.Delete("/{id}", h.DeleteExternalCalendar)
	})

	// Templates
	r.Route("/api/v1/templates", func(r chi.Router) {
		r.Get("/", h.ListTemplates)
		r.Post("/", h.CreateTemplate)
		r.Get("/{id}", h.GetTemplate)
		r.Put("/{id}", h.UpdateTemplate)
		r.Delete("/{id}", h.DeleteTemplate)
	})

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.Port),
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		log.Printf("meetings plugin listening on :%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("shutdown: %v", err)
	}
}
