package handlers

import (
	"crypto/sha1"
	"doctordoc/internal/service"
	"fmt"
	"net/http"
	"os"
)

type SubscriptionHandler struct {
	svc service.SubscriptionService
}

func NewSubscriptionHandler(svc service.SubscriptionService) *SubscriptionHandler {
	return &SubscriptionHandler{svc: svc}
}

func (h *SubscriptionHandler) YoomoneyWebhook(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	sha1Hash := r.FormValue("sha1_hash")
	label := r.FormValue("label") // Тут наш Email
	amount := r.FormValue("amount")

	rawStr := fmt.Sprintf("%s&%s&%s&%s&%s&%s&%s&%s&%s",
		r.FormValue("notification_type"), r.FormValue("operation_id"), amount,
		r.FormValue("currency"), r.FormValue("datetime"), r.FormValue("sender"),
		r.FormValue("codepro"), os.Getenv("YOO_KEY"), label)

	check := sha1.New()
	check.Write([]byte(rawStr))
	if fmt.Sprintf("%x", check.Sum(nil)) != sha1Hash {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	err := h.svc.ProcessPayment(label, amount)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *SubscriptionHandler) CheckLimit(w http.ResponseWriter, r *http.Request) {
	// Добавлен r.Context() в качестве первого аргумента
	allowed, _ := h.svc.IsAccessAllowed(r.Context(), r.RemoteAddr, r.Header.Get("X-Client-Fingerprint"))

	if !allowed {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	w.WriteHeader(http.StatusOK)
}