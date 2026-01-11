package handlers

import (
	"doctordoc/internal/models"
	"doctordoc/internal/service"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

type TelegramHandler struct {
	svc service.TelegramService
}

func NewTelegramHandler(s service.TelegramService) *TelegramHandler {
	return &TelegramHandler{svc: s}
}

func (h *TelegramHandler) SupportHandler(w http.ResponseWriter, r *http.Request) {
	var req models.SupportRequest

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("❌ Ошибка декодирования формы поддержки: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Неверный формат данных"})
		return
	}

	msg := fmt.Sprintf("🆘 ПОДДЕРЖКА\n\n👤 Имя: %s\n📧 Email: %s\n✈️ TG: %s\n📝 Текст: %s",
		req.Name, req.Email, req.Telegram, req.Text)

	// Отправляем асинхронно, чтобы не блокировать ответ
	go func() {
		_ = h.svc.SendMessage(msg)
	}()

	// ВСЕГДА отвечаем успехом клиенту, чтобы не было 500 ошибки
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"message": "Ваше сообщение принято",
	})
}