package service

import (
    "context"
    "fmt"
    "log"
    "time"
)

func (s *fileService) CanUpload(ctx context.Context, fingerprint string, ip string, fileSizeMB float64) (bool, error) {
    if ip == "" {
        return false, nil
    }

    // 1. ПРОВЕРКА УСТРОЙСТВ (Максимум 5 для всех)
    devices, _ := s.repo.GetDistinctDevicesCount(ctx, ip)
    if devices >= 5 {
        log.Printf("🛑 GLOBAL DEVICE LIMIT: IP %s already has %d devices", ip, devices)
        return false, fmt.Errorf("DEVICE_LIMIT_EXCEEDED")
    }

    // 2. Проверка подписки (Billing)
    if hasSub, _ := s.CheckOnlySubscription(ctx, ip, fingerprint); hasSub {
        return true, nil // Для платных дальше лимиты на файлы не смотрим
    }

    // 3. Лимит размера для Free
    if fileSizeMB > 5 {
        return false, fmt.Errorf("FILE_TOO_LARGE")
    }

    // 4. Лимит на файлы для Free (3 файла на 24 часа)
    machineID := s.GenerateHardwareHash(ip)
    usage, updatedAt, err := s.repo.GetUsageWithTime(ctx, machineID, ip)
    if err != nil {
        return true, nil
    }

    if usage >= 3 {
        if time.Since(updatedAt) < 24*time.Hour {
            log.Printf("🛑 FREE LIMIT ACTIVE: wait until %v", updatedAt.Add(24*time.Hour))
            return false, nil
        }
    }

    return true, nil
}