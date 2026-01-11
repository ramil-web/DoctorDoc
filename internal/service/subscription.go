package service

import (
	"context"
	"crypto/rand"
	"doctordoc/internal/repository"
	"encoding/hex"
	"fmt"
	"net/smtp"
	"os"
	"time"
)

// SubscriptionService описывает бизнес-логику работы с подписками и лимитами
type SubscriptionService interface {
	ProcessPayment(email, amountStr string) error
	IsAccessAllowed(ctx context.Context, ip, fp string) (bool, error)
	IncrementUsage(ctx context.Context, fp string) error
}

type subscriptionService struct {
	repo  repository.Repository
	tgSvc TelegramService
}

// NewSubscriptionService создает новый экземпляр сервиса
func NewSubscriptionService(repo repository.Repository, tgSvc TelegramService) SubscriptionService {
	return &subscriptionService{repo: repo, tgSvc: tgSvc}
}

// IsAccessAllowed проверяет, не превысил ли пользователь лимит в 3 файла
func (s *subscriptionService) IsAccessAllowed(ctx context.Context, ip, fp string) (bool, error) {
	// 1. Если есть активная подписка — разрешаем всегда
	hasSub, _ := s.repo.CheckActiveSubscription(ip, fp)
	if hasSub {
		return true, nil
	}

	// 2. Получаем текущее количество использований за сегодня
	usage, err := s.repo.GetUsageCount(ctx, fp)
	if err != nil {
		usage = 0 // Если записей нет, считаем что 0
	}

	// 3. ПРОВЕРКА ЛИМИТА:
	// Если уже 3 или больше, возвращаем false (доступ запрещен)
	if usage >= 3 {
		fmt.Printf("🛑 [LIMIT] Отказ в доступе для FP: %s (уже использовано: %d)\n", fp, usage)
		return false, nil
	}

	// ВАЖНО: Мы НЕ вызываем здесь IncrementUsage, чтобы не списывать попытку
	// просто за факт открытия страницы или загрузки файла.
	return true, nil
}

// IncrementUsage прибавляет +1 к счетчику использований в базе данных
func (s *subscriptionService) IncrementUsage(ctx context.Context, fp string) error {
	_, err := s.repo.IncrementUsage(ctx, fp)
	if err != nil {
		return fmt.Errorf("ошибка инкремента в репозитории: %w", err)
	}
	return nil
}

// ProcessPayment обрабатывает уведомления об оплате и создает подписку
func (s *subscriptionService) ProcessPayment(email, amountStr string) error {
	var amnt float64
	fmt.Sscanf(amountStr, "%f", &amnt)

	// Определение тарифа по сумме
	plan, duration := "Подписка", 24*time.Hour
	if amnt >= 45 && amnt <= 55 { plan, duration = "Разовый", 50*365*24*time.Hour }
	if amnt >= 95 && amnt <= 105 { plan, duration = "Сутки", 24*time.Hour }
	if amnt >= 450 && amnt <= 550 { plan, duration = "Месяц", 30*24*time.Hour }
	if amnt >= 2500 && amnt <= 3500 { plan, duration = "Год", 365*24*time.Hour }

	code := s.generateCode()
	if err := s.repo.CreateSubscription(email, plan, duration, code); err != nil {
		return err
	}

	// Уведомление в Телеграм и на Почту
	go s.tgSvc.SendMessage(fmt.Sprintf("💰 ОПЛАТА!\nEmail: %s\nСумма: %s руб.\nТариф: %s\nКод: %s", email, amountStr, plan, code))
	if email != "" {
		go s.sendEmail(email, code, plan)
	}
	return nil
}

// Вспомогательные методы
func (s *subscriptionService) generateCode() string {
	b := make([]byte, 4)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *subscriptionService) sendEmail(to, code, plan string) {
	from := os.Getenv("SMTP_EMAIL")
	msg := []byte(fmt.Sprintf("Subject: Ваш код DoctorDoc\r\n\r\nСпасибо за покупку тарифа \"%s\".\nВаш код активации: %s", plan, code))
	auth := smtp.PlainAuth("", from, os.Getenv("SMTP_PASSWORD"), os.Getenv("SMTP_HOST"))
	_ = smtp.SendMail(os.Getenv("SMTP_HOST")+":"+os.Getenv("SMTP_PORT"), auth, from, []string{to}, msg)
}