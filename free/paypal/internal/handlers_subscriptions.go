package internal

import (
	"context"
	"net/http"
	"time"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	sdk "github.com/nself-org/plugin-sdk"
)

func handleListSubscriptions(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		limit, offset := parsePagination(r)

		subscriptions, err := ListSubscriptions(ctx, pool, limit, offset)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, err)
			return
		}
		if subscriptions == nil {
			subscriptions = []Subscription{}
		}

		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"subscriptions": subscriptions,
			"limit":         limit,
			"offset":        offset,
		})
	}
}

func handleGetSubscription(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		id := chi.URLParam(r, "id")
		if id == "" {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "id is required"})
			return
		}

		subscription, err := GetSubscription(ctx, pool, id)
		if err != nil {
			sdk.Respond(w, http.StatusNotFound, map[string]string{"error": "subscription not found"})
			return
		}

		sdk.Respond(w, http.StatusOK, subscription)
	}
}

// --- Products handler --------------------------------------------------------

func handleListProducts(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		limit, offset := parsePagination(r)

		rows, err := pool.Query(ctx, `
			SELECT id, paypal_id, name, description, type, category, image_url, home_url, created_at, updated_at, source_account_id, synced_at
			FROM np_paypal_products
			ORDER BY created_at DESC
			LIMIT $1 OFFSET $2
		`, limit, offset)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, err)
			return
		}
		defer rows.Close()

		var products []Product
		for rows.Next() {
			var p Product
			if err := rows.Scan(&p.ID, &p.PayPalID, &p.Name, &p.Description, &p.Type, &p.Category,
				&p.ImageURL, &p.HomeURL, &p.CreatedAt, &p.UpdatedAt, &p.SourceAccountID, &p.SyncedAt); err != nil {
				sdk.Error(w, http.StatusInternalServerError, err)
				return
			}
			products = append(products, p)
		}
		if err := rows.Err(); err != nil {
			sdk.Error(w, http.StatusInternalServerError, err)
			return
		}
		if products == nil {
			products = []Product{}
		}

		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"products": products,
			"limit":    limit,
			"offset":   offset,
		})
	}
}

// --- Disputes handler --------------------------------------------------------

func handleListDisputes(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		limit, offset := parsePagination(r)

		disputes, err := ListDisputes(ctx, pool, limit, offset)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, err)
			return
		}
		if disputes == nil {
			disputes = []Dispute{}
		}

		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"disputes": disputes,
			"limit":    limit,
			"offset":   offset,
		})
	}
}

// --- Invoices handler --------------------------------------------------------
