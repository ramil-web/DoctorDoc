package handlers

import (
	"doctordoc/internal/models"
	"doctordoc/internal/service"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"github.com/go-chi/chi/v5"
)

type FileHandler struct {
	svc    service.FileService
	subSvc service.SubscriptionService
}

func NewFileHandler(svc service.FileService, subSvc service.SubscriptionService) *FileHandler {
	return &FileHandler{svc: svc, subSvc: subSvc}
}

func (h *FileHandler) Upload(w http.ResponseWriter, r *http.Request) {
    r.ParseMultipartForm(10 << 20)
    file, header, err := r.FormFile("file")
    if err != nil {
        http.Error(w, "INVALID_FILE", http.StatusBadRequest)
        return
    }
    defer file.Close()

    fp := r.FormValue("fingerprint") // Берем из формы
    fileSizeMB := float64(header.Size) / (1024 * 1024)

    // Просто проверяем, не списывая
    can, _ := h.svc.CanUpload(r.Context(), fp, fileSizeMB)
    if !can {
        w.WriteHeader(http.StatusForbidden)
        json.NewEncoder(w).Encode(map[string]string{"error": "LIMIT_EXCEEDED"})
        return
    }

    content, _ := io.ReadAll(file)
    // ВАЖНО: передаем fp четвертым аргументом
    id, err := h.svc.ProcessFile(r.Context(), header.Filename, content, fp)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    json.NewEncoder(w).Encode(map[string]string{"id": id})
}

func (h *FileHandler) Download(w http.ResponseWriter, r *http.Request) {
    id := chi.URLParam(r, "id")

    meta, err := h.svc.GetFileMeta(r.Context(), id)
    if err != nil {
        http.Error(w, "FILE_NOT_FOUND", http.StatusNotFound)
        return
    }

    // Защита от двойного списания
    if !meta.IsDownloaded && meta.Fingerprint != "" {
        err := h.subSvc.IncrementUsage(r.Context(), meta.Fingerprint)
        if err != nil {
            fmt.Printf("⚠️ Ошибка списания: %v\n", err)
        } else {
            meta.IsDownloaded = true
            // Убрали звездочку перед meta, передаем объект как есть
            h.svc.UpdateFileMeta(r.Context(), meta)
            fmt.Printf("📥 [LIMIT] Лимит списан для %s (файл %s)\n", meta.Fingerprint, id)
        }
    } else {
        fmt.Printf("ℹ️ [SKIP] Лимит не списывается (уже скачан или нет FP)\n")
    }

    w.Header().Set("Content-Disposition", "attachment; filename="+meta.OriginalName)
    http.ServeFile(w, r, meta.FilePath)
}

func (h *FileHandler) Fix(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID            string `json:"id"`
		LicenseNumber string `json:"license_number"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	fixReq := models.FixRequest{ID: req.ID, LicenseNumber: req.LicenseNumber}
	if err := h.svc.FixFile(r.Context(), fixReq); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"status": "completed"})
}


func (h *FileHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	status, errs, _ := h.svc.GetStatus(id)
	json.NewEncoder(w).Encode(map[string]interface{}{"status": status, "errors": errs})
}