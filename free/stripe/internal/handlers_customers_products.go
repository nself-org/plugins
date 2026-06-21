package internal

import (
	"net/http"
)

func (s *Server) handleListCustomers(w http.ResponseWriter, r *http.Request) {
	db := s.scopedDB(r)
	limit, offset := parsePagination(r)
	data, err := db.ListCustomers(r.Context(), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	total, _ := db.CountCustomers(r.Context())
	writeJSON(w, http.StatusOK, ListResponse{Data: data, Total: total, Limit: limit, Offset: offset})
}

func (s *Server) handleGetCustomer(w http.ResponseWriter, r *http.Request) {
	s.handleGetByID(w, r, "np_stripe_customers")
}

func (s *Server) handleListProducts(w http.ResponseWriter, r *http.Request) {
	db := s.scopedDB(r)
	limit, offset := parsePagination(r)
	data, err := db.ListProducts(r.Context(), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	total, _ := db.CountProducts(r.Context())
	writeJSON(w, http.StatusOK, ListResponse{Data: data, Total: total, Limit: limit, Offset: offset})
}

func (s *Server) handleGetProduct(w http.ResponseWriter, r *http.Request) {
	s.handleGetByID(w, r, "np_stripe_products")
}

func (s *Server) handleListPrices(w http.ResponseWriter, r *http.Request) {
	db := s.scopedDB(r)
	limit, offset := parsePagination(r)
	data, err := db.ListPrices(r.Context(), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	total, _ := db.CountPrices(r.Context())
	writeJSON(w, http.StatusOK, ListResponse{Data: data, Total: total, Limit: limit, Offset: offset})
}

func (s *Server) handleGetPrice(w http.ResponseWriter, r *http.Request) {
	s.handleGetByID(w, r, "np_stripe_prices")
}
