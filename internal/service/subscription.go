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
	IncrementUsage(ctx context.Context, fp string, ip string) error
	// ИСПРАВЛЕНО: теперь принимает IP, чтобы связать разные браузеры в БД
	IncrementUsageWithCount(ctx context.Context, fp string, ip string) (int, error)
}

type subscriptionService struct {
	repo  repository.Repository
	tgSvc TelegramService
}

// NewSubscriptionService создает новый экземпляр сервиса
func NewSubscriptionService(repo repository.Repository, tgSvc TelegramService) SubscriptionService {
	return &subscriptionService{repo: repo, tgSvc: tgSvc}
}

// IsAccessAllowed проверяет лимит (3 файла в сутки) по связке IP + FP
func (s *subscriptionService) IsAccessAllowed(ctx context.Context, ip, fp string) (bool, error) {
	// 1. Проверка активной подписки (по IP или по FP)
	hasSub, _ := s.repo.CheckActiveSubscription(ip, fp)
	if hasSub {
		return true, nil
	}

	// 2. Получаем суммарное количество использований
	// В репозитории запрос должен быть: WHERE (fingerprint = $1 OR ip_address = $2)
	usage, err := s.repo.GetUsageCount(ctx, fp, ip)
	if err != nil {
		usage = 0
	}

	// 3. Блокировка
	if usage >= 3 {
		fmt.Printf("🛑 [LIMIT] Отказ: FP: %s | IP: %s (Всего: %d)\n", fp, ip, usage)
		return false, nil
	}

	return true, nil
}

// IncrementUsage прибавляет +1 (используется там, где не важен результат счетчика)
func (s *subscriptionService) IncrementUsage(ctx context.Context, fp string, ip string) error {
	_, err := s.repo.IncrementUsage(ctx, fp, ip)
	return err
}

// IncrementUsageWithCount прибавляет +1 и возвращает новое значение (для логов)
// ИСПРАВЛЕНО: теперь передает реальный IP в репозиторий
func (s *subscriptionService) IncrementUsageWithCount(ctx context.Context, fp string, ip string) (int, error) {
	fmt.Printf("📡 [SERVICE] Инкремент лимита: FP=%s, IP=%s\n", fp, ip)
	return s.repo.IncrementUsage(ctx, fp, ip)
}

// ProcessPayment обрабатывает оплату
func (s *subscriptionService) ProcessPayment(email, amountStr string) error {
	var amnt float64
	fmt.Sscanf(amountStr, "%f", &amnt)

	plan, duration := "Подписка", 24*time.Hour
	if amnt >= 45 && amnt <= 55 {
		plan, duration = "Разовый", 50*365*24*time.Hour
	}
	if amnt >= 95 && amnt <= 105 {
		plan, duration = "Сутки", 24*time.Hour
	}
	if amnt >= 450 && amnt <= 550 {
		plan, duration = "Месяц", 30*24*time.Hour
	}
	if amnt >= 2500 && amnt <= 3500 {
		plan, duration = "Год", 365*24*time.Hour
	}

	code := s.generateCode()
	if err := s.repo.CreateSubscription(email, plan, duration, code); err != nil {
		return err
	}

	go s.tgSvc.SendMessage(fmt.Sprintf("💰 ОПЛАТА!\nEmail: %s\nСумма: %s руб.\nТариф: %s\nКод: %s", email, amountStr, plan, code))
	if email != "" {
		go s.sendEmail(email, code, plan)
	}
	return nil
}

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