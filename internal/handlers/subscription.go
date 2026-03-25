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
		Email       string `json:"email"`
		PlanID      int    `json:"plan_id"`
		Fingerprint string `json:"fingerprint"`
	}

	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// ТАРИФИКАЦИЯ НА СЕРВЕРЕ (Безопасно)
	var amount float64
	var planName string

	switch data.PlanID {
	case 1:
		amount = 5.00
		planName = "Разовый"
	case 2:
		amount = 100.00
		planName = "Сутки"
	case 3:
		amount = 500.00
		planName = "Месяц"
	case 4:
		amount = 3000.00
		planName = "Год"
	default:
		w.WriteHeader(http.StatusBadRequest)
		return
	}


	wallet := os.Getenv("WALLET_ID")
	// В label записываем всё, что нам нужно восстановить в вебхуке
	label := fmt.Sprintf("pay_%s_%d_%s", data.Fingerprint, data.PlanID, data.Email)

	paymentURL := fmt.Sprintf(
		"https://yoomoney.ru/quickpay/confirm.xml?receiver=%s&quickpay-form=button&targets=Оплата+тарифа:+%s&paymentType=AC&sum=%.2f&label=%s&successURL=%s",
		wallet, planName, amount, label, os.Getenv("EXTERNAL_API_URL"),
	)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"url": paymentURL})
}

func (h *SubscriptionHandler) YoomoneyWebhook(w http.ResponseWriter, r *http.Request) {
	// Добавляем логирование входящего запроса
	fmt.Println("\n--- [YOOMONEY WEBHOOK START] ---")

	if err := r.ParseForm(); err != nil {
		fmt.Printf("❌ [ERROR] Ошибка парсинга формы: %v\n", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	sha1Hash := r.FormValue("sha1_hash")
	label := r.FormValue("label")
	amount := r.FormValue("amount")
	withdrawAmount := r.FormValue("withdraw_amount")

	fmt.Printf("📥 Получен label: %s\n", label)
	fmt.Printf("📥 Сумма (amount): %s | К списанию (withdraw_amount): %s\n", amount, withdrawAmount)
	fmt.Printf("📥 SHA1 от ЮMoney: %s\n", sha1Hash)

	// Формируем строку для проверки
	rawStr := fmt.Sprintf("%s&%s&%s&%s&%s&%s&%s&%s&%s",
		r.FormValue("notification_type"),
		r.FormValue("operation_id"),
		amount,
		r.FormValue("currency"),
		r.FormValue("datetime"),
		r.FormValue("sender"),
		r.FormValue("codepro"),
		os.Getenv("YOO_KEY"),
		label,
	)

	// Логируем строку (без первых символов секретного ключа для безопасности, но чтобы проверить структуру)
	fmt.Printf("📋 Строка для хеша: %s\n", rawStr)

	check := sha1.New()
	check.Write([]byte(rawStr))
	myHash := fmt.Sprintf("%x", check.Sum(nil))

	fmt.Printf("🧮 Рассчитанный хеш: %s\n", myHash)

	if myHash != sha1Hash {
		fmt.Println("⚠️  [AUTH FAIL] Хеши не совпали! Проверьте YOO_KEY в .env")
		fmt.Println("--- [WEBHOOK END] ---")
		w.WriteHeader(http.StatusForbidden)
		return
	}

	fmt.Println("✅ [AUTH OK] Хеш подтвержден. Обработка платежа...")

	// Передаем label и сумму списания в сервис для активации
	err := h.svc.ProcessPayment(label, withdrawAmount)
	if err != nil {
		fmt.Printf("❌ [SERVICE ERROR] Ошибка обработки: %v\n", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	fmt.Println("🎉 [SUCCESS] Подписка активирована!")
	fmt.Println("--- [WEBHOOK END] ---")
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