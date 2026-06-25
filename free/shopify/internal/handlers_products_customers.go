package internal

import (
	"github.com/go-chi/chi/v5"
	"net/http"
)

func handleListShops(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		shop, err := db.GetShop(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		var data []interface{}
		if shop != nil {
			data = append(data, shop)
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"data":  data,
			"total": len(data),
		})
	}
}

func handleListProducts(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit, offset := pagination(r)
		products, err := db.ListProducts(r.Context(), limit, offset)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		total, _ := db.CountProducts(r.Context())
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"data": products, "total": total, "limit": limit, "offset": offset,
		})
	}
}

func handleGetProduct(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		product, err := db.GetProduct(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if product == nil {
			writeError(w, http.StatusNotFound, "Product not found")
			return
		}
		variants, _ := db.GetProductVariants(r.Context(), product.ID)
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"product": product, "variants": variants,
		})
	}
}

func handleListVariants(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		limit, offset := pagination(r)
		variants, err := db.ListVariants(r.Context(), limit, offset)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		total, _ := db.CountVariants(r.Context())
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"data": variants, "total": total, "limit": limit, "offset": offset,
		})
	}
}

func handleListCollections(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit, offset := pagination(r)
		collections, err := db.ListCollections(r.Context(), limit, offset)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		total, _ := db.CountCollections(r.Context())
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"data": collections, "total": total, "limit": limit, "offset": offset,
		})
	}
}

func handleListCustomers(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit, offset := pagination(r)
		customers, err := db.ListCustomers(r.Context(), limit, offset)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		total, _ := db.CountCustomers(r.Context())
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"data": customers, "total": total, "limit": limit, "offset": offset,
		})
	}
}

func handleGetCustomer(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		customer, err := db.GetCustomer(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if customer == nil {
			writeError(w, http.StatusNotFound, "Customer not found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"customer": customer})
	}
}

func handleListOrders(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit, offset := pagination(r)
		status := r.URL.Query().Get("status")
		orders, err := db.ListOrders(r.Context(), status, limit, offset)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		total, _ := db.CountOrders(r.Context(), status)
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"data": orders, "total": total, "limit": limit, "offset": offset,
		})
	}
}

func handleGetOrder(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		order, err := db.GetOrder(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if order == nil {
			writeError(w, http.StatusNotFound, "Order not found")
			return
		}
		items, _ := db.GetOrderItems(r.Context(), order.ID)
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"order": order, "items": items,
		})
	}
}

