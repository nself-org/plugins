package internal

import (
	"context"
	"net/http"
	"time"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	sdk "github.com/nself-org/plugin-sdk"
)

func handleListTransactions(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		limit, offset := parsePagination(r)

		transactions, err := ListTransactions(ctx, pool, limit, offset)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, err)
			return
		}
		if transactions == nil {
			transactions = []Transaction{}
		}

		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"transactions": transactions,
			"limit":        limit,
			"offset":       offset,
		})
	}
}

func handleGetTransaction(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		id := chi.URLParam(r, "id")
		if id == "" {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "id is required"})
			return
		}

		transaction, err := GetTransaction(ctx, pool, id)
		if err != nil {
			sdk.Respond(w, http.StatusNotFound, map[string]string{"error": "transaction not found"})
			return
		}

		sdk.Respond(w, http.StatusOK, transaction)
	}
}

// --- Order handlers ----------------------------------------------------------

func handleListOrders(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		limit, offset := parsePagination(r)

		orders, err := ListOrders(ctx, pool, limit, offset)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, err)
			return
		}
		if orders == nil {
			orders = []Order{}
		}

		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"orders": orders,
			"limit":  limit,
			"offset": offset,
		})
	}
}

func handleGetOrder(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		id := chi.URLParam(r, "id")
		if id == "" {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "id is required"})
			return
		}

		order, err := GetOrder(ctx, pool, id)
		if err != nil {
			sdk.Respond(w, http.StatusNotFound, map[string]string{"error": "order not found"})
			return
		}

		sdk.Respond(w, http.StatusOK, order)
	}
}

// --- Subscription handlers ---------------------------------------------------
