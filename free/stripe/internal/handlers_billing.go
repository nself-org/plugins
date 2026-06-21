package internal

import (
	"net/http"
)

func (s *Server) handleListSubscriptions(w http.ResponseWriter, r *http.Request) {
	db := s.scopedDB(r)
	limit, offset := parsePagination(r)
	status := r.URL.Query().Get("status")
	data, err := db.ListSubscriptions(r.Context(), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	total, _ := db.CountSubscriptions(r.Context(), status)
	writeJSON(w, http.StatusOK, ListResponse{Data: data, Total: total, Limit: limit, Offset: offset})
}

func (s *Server) handleGetSubscription(w http.ResponseWriter, r *http.Request) {
	s.handleGetByID(w, r, "np_stripe_subscriptions")
}

func (s *Server) handleListInvoices(w http.ResponseWriter, r *http.Request) {
	db := s.scopedDB(r)
	limit, offset := parsePagination(r)
	status := r.URL.Query().Get("status")
	data, err := db.ListInvoices(r.Context(), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	total, _ := db.CountInvoices(r.Context(), status)
	writeJSON(w, http.StatusOK, ListResponse{Data: data, Total: total, Limit: limit, Offset: offset})
}

func (s *Server) handleGetInvoice(w http.ResponseWriter, r *http.Request) {
	s.handleGetByID(w, r, "np_stripe_invoices")
}

func (s *Server) handleListPaymentIntents(w http.ResponseWriter, r *http.Request) {
	db := s.scopedDB(r)
	limit, offset := parsePagination(r)
	data, err := db.ListPaymentIntents(r.Context(), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	total, _ := db.CountPaymentIntents(r.Context())
	writeJSON(w, http.StatusOK, ListResponse{Data: data, Total: total, Limit: limit, Offset: offset})
}

func (s *Server) handleGetPaymentIntent(w http.ResponseWriter, r *http.Request) {
	s.handleGetByID(w, r, "np_stripe_payment_intents")
}

func (s *Server) handleListCharges(w http.ResponseWriter, r *http.Request) {
	db := s.scopedDB(r)
	limit, offset := parsePagination(r)
	data, err := db.ListCharges(r.Context(), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	total, _ := db.CountCharges(r.Context())
	writeJSON(w, http.StatusOK, ListResponse{Data: data, Total: total, Limit: limit, Offset: offset})
}

func (s *Server) handleGetCharge(w http.ResponseWriter, r *http.Request) {
	s.handleGetByID(w, r, "np_stripe_charges")
}

func (s *Server) handleListRefunds(w http.ResponseWriter, r *http.Request) {
	db := s.scopedDB(r)
	limit, offset := parsePagination(r)
	data, err := db.ListRefunds(r.Context(), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	total, _ := db.CountRefunds(r.Context())
	writeJSON(w, http.StatusOK, ListResponse{Data: data, Total: total, Limit: limit, Offset: offset})
}

func (s *Server) handleGetRefund(w http.ResponseWriter, r *http.Request) {
	s.handleGetByID(w, r, "np_stripe_refunds")
}

func (s *Server) handleListCoupons(w http.ResponseWriter, r *http.Request) {
	db := s.scopedDB(r)
	limit, offset := parsePagination(r)
	data, err := db.ListCoupons(r.Context(), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	total, _ := db.CountCoupons(r.Context())
	writeJSON(w, http.StatusOK, ListResponse{Data: data, Total: total, Limit: limit, Offset: offset})
}

func (s *Server) handleGetCoupon(w http.ResponseWriter, r *http.Request) {
	s.handleGetByID(w, r, "np_stripe_coupons")
}

func (s *Server) handleListBalanceTransactions(w http.ResponseWriter, r *http.Request) {
	db := s.scopedDB(r)
	limit, offset := parsePagination(r)
	data, err := db.ListBalanceTransactions(r.Context(), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	total, _ := db.CountBalanceTransactions(r.Context())
	writeJSON(w, http.StatusOK, ListResponse{Data: data, Total: total, Limit: limit, Offset: offset})
}

func (s *Server) handleGetBalanceTransaction(w http.ResponseWriter, r *http.Request) {
	s.handleGetByID(w, r, "np_stripe_balance_transactions")
}
