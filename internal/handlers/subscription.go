package handlers

import (
	"crypto/sha1"
	"doctordoc/internal/service"
	"encoding/json"
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

func (h *SubscriptionHandler) CreatePayment(w http.ResponseWriter, r *http.Request) {
	var data struct {
		Email       string  `json:"email"`
		PlanID      int     `json:"plan_id"`
		PlanName    string  `json:"plan_name"`
		Amount      float64 `json:"amount"`
		Fingerprint string  `json:"fingerprint"`
	}

	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	wallet := os.Getenv("WALLET_ID")
	label := fmt.Sprintf("pay_%s_%d_%s", data.Fingerprint, data.PlanID, data.Email)

	paymentURL := fmt.Sprintf(
		"https://yoomoney.ru/quickpay/confirm.xml?receiver=%s&quickpay-form=button&targets=Оплата+тарифа:+%s&paymentType=AC&sum=%.2f&label=%s&successURL=%s",
		wallet, data.PlanName, data.Amount, label, os.Getenv("EXTERNAL_API_URL"),
	)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"url": paymentURL})
}

func (h *SubscriptionHandler) YoomoneyWebhook(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	sha1Hash := r.FormValue("sha1_hash")
	label := r.FormValue("label")
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

	_ = h.svc.ProcessPayment(label, amount)
	w.WriteHeader(http.StatusOK)
}

func (h *SubscriptionHandler) CheckLimit(w http.ResponseWriter, r *http.Request) {
	allowed, _ := h.svc.IsAccessAllowed(r.Context(), r.RemoteAddr, r.Header.Get("X-Client-Fingerprint"))
	if !allowed {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	w.WriteHeader(http.StatusOK)
}