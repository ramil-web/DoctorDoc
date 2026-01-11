package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"

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

func runMigrations(db *sql.DB) {
	// Проверяем два возможных пути к файлу
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
	if dbURL == "" {
		log.Println("⚠️  ВНИМАНИЕ: .env не загружен или DB_URL пуст.")
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("DB Error: %v", err)
	}

	// ЗАПУСК МИГРАЦИЙ ПРИ СТАРТЕ
	runMigrations(db)

	rdb := redis.NewClient(&redis.Options{Addr: os.Getenv("REDIS_URL")})

	repo := repository.NewRepository(db)
	tgSvc := service.NewTelegramService()
	fileSvc := service.NewFileService(repo, rdb)
	subSvc := service.NewSubscriptionService(repo, tgSvc)

	// ИСПРАВЛЕНО: Теперь передаем два аргумента (fileSvc и subSvc)
	fileHandler := handlers.NewFileHandler(fileSvc, subSvc)

	subHandler := handlers.NewSubscriptionHandler(subSvc)
	tgHandler := handlers.NewTelegramHandler(tgSvc)
	authHandler := handlers.NewAuthHandler(fileSvc, subSvc)

	go fileSvc.StartWorker(context.Background())

	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "OPTIONS", "PUT", "DELETE"},
		AllowedHeaders:   []string{"Accept", "Content-Type", "Content-Length", "Accept-Encoding", "X-CSRF-Token", "Authorization", "X-API-KEY", "X-Client-Fingerprint", "ngrok-skip-browser-warning"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/support", tgHandler.SupportHandler)
		r.Post("/webhook/yoomoney", subHandler.YoomoneyWebhook)

		r.Group(func(r chi.Router) {
			r.Use(authHandler.AuthMiddleware)
			r.Use(authHandler.LimitMiddleware)

			r.Post("/upload", fileHandler.Upload)
			r.Post("/fix", fileHandler.Fix)
		})

		r.Get("/status/{id}", fileHandler.GetStatus)
		r.Get("/download/{id}", fileHandler.Download)
		r.Get("/check-limit", subHandler.CheckLimit)
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("🚀 Server on port %s", port)
	http.ListenAndServe(":"+port, r)
}