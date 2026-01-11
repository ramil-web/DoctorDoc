package handlers

import (
    "doctordoc/internal/service"
    "encoding/json"
    "fmt"
    "net/http"
    "os"
)

type AuthHandler struct {
    fileSvc service.FileService
    subSvc  service.SubscriptionService
}

func NewAuthHandler(fs service.FileService, ss service.SubscriptionService) *AuthHandler {
    return &AuthHandler{fileSvc: fs, subSvc: ss}
}

func (h *AuthHandler) AuthMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
       secret := os.Getenv("API_SECRET_TOKEN")
       clientKey := r.Header.Get("X-API-KEY")

       if clientKey == "" {
          clientKey = r.URL.Query().Get("key")
       }

       if clientKey == "" || clientKey == "undefined" || clientKey != secret {
          fmt.Printf("[AUTH] REJECTED. Got: '%s', Expected: '%s'\n", clientKey, secret)
          w.Header().Set("Content-Type", "application/json")
          w.WriteHeader(http.StatusUnauthorized)
          json.NewEncoder(w).Encode(map[string]string{"error": "Unauthorized"})
          return
       }

       next.ServeHTTP(w, r)
    })
}

func (h *AuthHandler) LimitMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
       fp := r.Header.Get("X-Client-Fingerprint")
       if fp == "" {
          next.ServeHTTP(w, r)
          return
       }

       allowed, err := h.subSvc.IsAccessAllowed(r.Context(), r.RemoteAddr, fp)
       if err == nil && !allowed {
          w.Header().Set("Content-Type", "application/json")
          w.WriteHeader(http.StatusForbidden)
          json.NewEncoder(w).Encode(map[string]string{"error": "LIMIT_EXCEEDED"})
          return
       }
       next.ServeHTTP(w, r)
    })
}