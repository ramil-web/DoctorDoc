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

	// Декодируем тело запроса
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("❌ Ошибка декодирования формы поддержки: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Формируем сообщение
	msg := fmt.Sprintf(
		"🆘 <b>НОВАЯ ЗАЯВКА В ПОДДЕРЖКУ</b>\n\n👤 <b>Имя:</b> %s\n📧 <b>Email:</b> %s\n✈️ <b>TG:</b> %s\n📝 <b>Текст:</b> %s",
		req.Name, req.Email, req.Telegram, req.Text,
	)

	// Отправляем в Telegram через сервис (токены бота лежат в .env на сервере)
	go func() {
		_ = h.svc.SendMessage(msg)
	}()

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}