package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log" // Добавлено
	"net/http"
	"os"
)

type TelegramService interface {
	SendMessage(text string) error
}

type telegramService struct {
	token  string
	chatID string
}

func NewTelegramService() TelegramService {
	token := os.Getenv("TG_BOT_TOKEN")
	chatID := os.Getenv("TG_CHAT_ID")

	if token == "" || chatID == "" {
		log.Println("‼️  ОШИБКА: Telegram ключи не найдены в .env!")
	} else {
		log.Printf("✅ Telegram инициализирован для ID: %s", chatID)
	}

	return &telegramService{
		token:  token,
		chatID: chatID,
	}
}

func (s *telegramService) SendMessage(text string) error {
	if s.token == "" || s.chatID == "" {
		log.Println("❌ Пропуск отправки в TG: ключи не загружены")
		return nil
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", s.token)
	payload := map[string]string{
		"chat_id":    s.chatID,
		"text":       text,
		"parse_mode": "HTML",
	}

	body, _ := json.Marshal(payload)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(body))
	if err != nil {
		log.Printf("❌ Ошибка сети TG: %v", err)
		return nil
	}
	defer resp.Body.Close()

	return nil
}