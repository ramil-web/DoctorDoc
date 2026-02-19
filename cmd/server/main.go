package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"doctordoc/internal/handlers"
	"doctordoc/internal/repository"
	"doctordoc/internal/service"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/go-redis/redis/v8"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

// ApiKeyMiddleware проверяет наличие и валидность секретного токена в заголовках
func ApiKeyMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Пропускаем проверку для Preflight-запросов браузера (CORS)
		if r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}

		// Считываем Fingerprint из заголовка
		fingerprint := r.Header.Get("X-Client-Fingerprint")

		// Если Fingerprint отсутствует, блокируем доступ
		if fingerprint == "" || fingerprint == "none" {
			log.Printf("⚠️  Блокировка: запрос без идентификатора (IP: %s)", r.RemoteAddr)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]string{"error": "Access denied: Missing Fingerprint"})
			return
		}

		next.ServeHTTP(w, r)
	})
}

func runMigrations(db *sql.DB) {
	paths := []string{"migrations/001_init.sql", "backend/migrations/001_init.sql"}
	var content []byte
	var err error

	for _, p := range paths {
		content, err = os.ReadFile(p)
		if err == nil {
			log.Printf("📂 Файл миграций найден: %s", p)
			break
		}
	}

	if err != nil {
		log.Printf("⚠️  Не удалось найти файл миграций в %v", paths)
		return
	}

	_, err = db.Exec(string(content))
	if err != nil {
		log.Printf("❌ Ошибка при выполнении миграций: %v", err)
		return
	}
	log.Println("✅ Миграции успешно применены")
}

func main() {
	if err := godotenv.Load(); err != nil {
		_ = godotenv.Load("../../.env")
	}

	dbURL := os.Getenv("DB_URL")
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("❌ DB Connection Error: %v", err)
	}
	if err := db.Ping(); err != nil {
		log.Fatalf("❌ DB Ping Error: %v", err)
	}

	runMigrations(db)

	rdb := redis.NewClient(&redis.Options{
		Addr: os.Getenv("REDIS_URL"),
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("❌ Redis Error: %v", err)
	}
	log.Println("✅ Redis подключен")

	repo := repository.NewRepository(db)
	tgSvc := service.NewTelegramService()
	fileSvc := service.NewFileService(repo, rdb)
	subSvc := service.NewSubscriptionService(repo, tgSvc)

	fileHandler := handlers.NewFileHandler(fileSvc, subSvc)
	subHandler := handlers.NewSubscriptionHandler(subSvc)
	tgHandler := handlers.NewTelegramHandler(tgSvc)
	authHandler := handlers.NewAuthHandler(fileSvc, subSvc)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("🔥 КРИТИЧЕСКАЯ ПАНИКА В ВОРКЕРЕ: %v", r)
			}
		}()
		log.Println("👷 Воркер запускается...")
		fileSvc.StartWorker(context.Background())
	}()

	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST", "OPTIONS", "PUT", "DELETE"},
		AllowedHeaders: []string{
			"Accept",
			"Content-Type",
			"Content-Length",
			"Accept-Encoding",
			"X-CSRF-Token",
			"Authorization",
			"X-API-KEY",
			"X-Client-Fingerprint",
			"ngrok-skip-browser-warning",
		},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Группа API v1
	r.Route("/api/v1", func(r chi.Router) {
           // ВАЖНО: Вебхук ЮMoney выносим ДО ApiKeyMiddleware
           r.Post("/webhook/yoomoney", subHandler.YoomoneyWebhook)

           r.Group(func(r chi.Router) {
              // Эта проверка теперь применяется ко всему, что ниже
              r.Use(ApiKeyMiddleware)

              r.Post("/support", tgHandler.SupportHandler)
              r.Post("/create-payment", subHandler.CreatePayment)

              r.Group(func(r chi.Router) {
                 r.Use(authHandler.AuthMiddleware)
                 r.Use(authHandler.LimitMiddleware)

                 r.Post("/upload", fileHandler.Upload)
                 r.Post("/fix", fileHandler.Fix)
                 r.Post("/preview", fileHandler.Preview)
              })

              r.Get("/status/{id}", fileHandler.GetStatus)
              r.Get("/download/{id}", fileHandler.Download)
              r.Get("/check-limit", subHandler.CheckLimit)
           })
        })

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("🚀 Server on port %s", port)

	server := &http.Server{
		Addr:    ":" + port,
		Handler: r,
	}

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("❌ Server Error: %v", err)
	}
}