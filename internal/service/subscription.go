package service

import (
	"context"
	"crypto/rand"
	"database/sql"
	"doctordoc/internal/models"
	"doctordoc/internal/repository"
	"errors"
	"fmt"
	"net/smtp"
	"os"
	"strings"
	"time"
)

// Ошибки бизнес-логики
var (
	ErrInvalidCode      = errors.New("неверный или неактивный код")
	ErrAlreadyActivated = errors.New("код уже использован на другом устройстве")
)

type SubscriptionService interface {
	ProcessPayment(email, amountStr string) error
	IsAccessAllowed(ctx context.Context, ip, fp string) (bool, error)
	IncrementUsage(ctx context.Context, fp string, ip string) error
	IncrementUsageWithCount(ctx context.Context, fp string, ip string) (int, error)
	ActivateKey(ctx context.Context, key, fp string) error
}

type subscriptionService struct {
	repo  repository.Repository
	tgSvc TelegramService
}

func NewSubscriptionService(repo repository.Repository, tgSvc TelegramService) SubscriptionService {
	return &subscriptionService{repo: repo, tgSvc: tgSvc}
}

func (s *subscriptionService) ActivateKey(ctx context.Context, key, fp string) error {
	var license models.Subscription
	var err error

	license, err = s.repo.ActivateLicense(ctx, key, fp)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrInvalidCode
		}
		if err.Error() == "limit_exceeded" {
			return errors.New("максимум 5 устройств на один код")
		}
		return err
	}

	fmt.Printf("🔑 [SERVICE] Активация: Key=%s -> FP=%s (Тариф: %s)\n", key, fp, license.PlanType)
	go s.tgSvc.SendMessage(fmt.Sprintf("🔑 АКТИВАЦИЯ!\nКлюч: %s\nDevice: %s\nТариф: %s", key, fp, license.PlanType))

	return nil
}

func (s *subscriptionService) IsAccessAllowed(ctx context.Context, ip, fp string) (bool, error) {
	hasSub, _ := s.repo.CheckActiveSubscription(ip, fp)
	if hasSub {
		return true, nil
	}

	usage, err := s.repo.GetUsageCount(ctx, fp, ip)
	if err != nil {
		usage = 0
	}

	if usage >= 3 {
		fmt.Printf("🛑 [LIMIT] Отказ: FP: %s | IP: %s (Всего: %d)\n", fp, ip, usage)
		return false, nil
	}

	return true, nil
}

func (s *subscriptionService) IncrementUsage(ctx context.Context, fp string, ip string) error {
	_, err := s.repo.IncrementUsage(ctx, fp, ip)
	return err
}

func (s *subscriptionService) IncrementUsageWithCount(ctx context.Context, fp string, ip string) (int, error) {
	fmt.Printf("📡 [SERVICE] Инкремент лимита: FP=%s, IP=%s\n", fp, ip)
	return s.repo.IncrementUsage(ctx, fp, ip)
}

func (s *subscriptionService) ProcessPayment(labelData, amountStr string) error {
	// ИСПРАВЛЕНО: Надежный парсинг через Split вместо Sscanf
	parts := strings.Split(labelData, "_")
	if len(parts) < 4 {
		return fmt.Errorf("некорректный label: %s", labelData)
	}
	email := parts[3]

	var amnt float64
	fmt.Sscanf(amountStr, "%f", &amnt)

	plan, duration := "Подписка", 24*time.Hour

	// Логика тарифов остается прежней
	if amnt >= 0.5 && amnt <= 7 { // Расширил диапазон для теста 5р
		plan, duration = "Разовый", 50*365*24*time.Hour
	} else if amnt >= 95 && amnt <= 105 {
		plan, duration = "Сутки", 24*time.Hour
	} else if amnt >= 450 && amnt <= 550 {
		plan, duration = "Месяц", 30*24*time.Hour
	} else if amnt >= 2500 && amnt <= 3500 {
		plan, duration = "Год", 365*24*time.Hour
	}

	code := s.generateCode()

	if err := s.repo.CreateSubscription(email, plan, duration, code); err != nil {
		return err
	}

	go s.tgSvc.SendMessage(fmt.Sprintf("💰 ОПЛАТА!\nEmail: %s\nСумма: %s руб.\nТариф: %s\nКод: %s", email, amountStr, plan, code))

	if email != "" {
		fmt.Printf("📧 [MAIL] Попытка отправки на: %s\n", email)
		go s.sendEmail(email, code, plan)
	}
	return nil
}

func (s *subscriptionService) generateCode() string {
	b := make([]byte, 16)
    	_, _ = rand.Read(b)
    	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func (s *subscriptionService) sendEmail(to, code, plan string) {
	from := os.Getenv("SMTP_EMAIL")
	host := os.Getenv("SMTP_HOST")
	port := os.Getenv("SMTP_PORT")
	password := os.Getenv("SMTP_PASSWORD")

	// ИСПРАВЛЕНО: Улучшенные заголовки письма, чтобы не попасть в спам
	headerFrom := fmt.Sprintf("From: DoctorDoc <%s>\r\n", from)
	headerTo := fmt.Sprintf("To: %s\r\n", to)
	subject := "Subject: Ваш код активации DoctorDoc\r\n"
	mime := "MIME-version: 1.0;\nContent-Type: text/plain; charset=\"UTF-8\";\n\n"

	body := fmt.Sprintf("Здравствуйте!\n\nСпасибо за покупку тарифа \"%s\".\nВаш персональный код активации: %s\n\nВведите его в приложении для снятия ограничений.", plan, code)

	msg := []byte(headerFrom + headerTo + subject + mime + body)
	auth := smtp.PlainAuth("", from, password, host)

	err := smtp.SendMail(host+":"+port, auth, from, []string{to}, msg)
	if err != nil {
		fmt.Printf("❌ [MAIL ERROR] Не удалось отправить письмо на %s: %v\n", to, err)
	} else {
		fmt.Printf("✅ [MAIL SUCCESS] Письмо отправлено на %s\n", to)
	}
}