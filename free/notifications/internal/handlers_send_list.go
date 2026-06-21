package internal

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	sdk "github.com/nself-org/plugin-sdk"
)

func handleSendNotification(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req SendNotificationRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}

		if req.Channel == "" {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "channel is required"})
			return
		}
		if req.Recipient == "" {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "recipient is required"})
			return
		}

		// Resolve template content if a template name is provided.
		subject := ""
		body := ""
		if req.Template != "" {
			subject = req.Template
			body = req.Template
		}

		var result ChannelResult
		switch req.Channel {
		case "email":
			result = SendEmail(req.Recipient, subject, body)
		case "push":
			result = SendPush(req.Recipient, subject, body)
		case "sms":
			result = SendSMS(req.Recipient, body)
		default:
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "unsupported channel: " + req.Channel})
			return
		}

		// Record the notification in the database.
		id := uuid.New().String()
		status := "sent"
		var sentAt *time.Time
		var errMsg *string

		if result.Success {
			now := time.Now().UTC()
			sentAt = &now
		} else {
			status = "failed"
			errMsg = result.Error
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		_ = InsertNotification(ctx, pool, id, req.Channel, req.Recipient, req.Template, req.Data, status, sentAt, errMsg)

		httpStatus := http.StatusOK
		if !result.Success {
			httpStatus = http.StatusBadGateway
		}

		sdk.Respond(w, httpStatus, map[string]interface{}{
			"id":      id,
			"channel": result.Channel,
			"success": result.Success,
			"error":   result.Error,
		})
	}
}

func handleListNotifications(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		channel := r.URL.Query().Get("channel")
		status := r.URL.Query().Get("status")
		recipient := r.URL.Query().Get("recipient")

		limit := 50
		if v := r.URL.Query().Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
				limit = n
			}
		}

		offset := 0
		if v := r.URL.Query().Get("offset"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n >= 0 {
				offset = n
			}
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		notifications, err := ListNotifications(ctx, pool, channel, status, recipient, limit, offset)
		if err != nil {
			sdk.Respond(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		if notifications == nil {
			notifications = []Notification{}
		}

		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"notifications": notifications,
			"limit":         limit,
			"offset":        offset,
		})
	}
}

func handleGetNotification(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if id == "" {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "id is required"})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		notification, err := GetNotification(ctx, pool, id)
		if err != nil {
			sdk.Respond(w, http.StatusNotFound, map[string]string{"error": "notification not found"})
			return
		}

		sdk.Respond(w, http.StatusOK, notification)
	}
}
