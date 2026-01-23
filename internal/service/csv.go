package service

import (
    "doctordoc/internal/models"
    "encoding/csv"
    "os"
    "strings"
    "sync"
)

func (s *fileService) processCSV(meta *models.FileMetadata, save bool, userReq models.FixRequest) error {
    content, err := os.ReadFile(meta.FilePath)
    if err != nil { return err }
    raw := string(content); sep := ';'; if strings.Count(raw, ",") > strings.Count(raw, ";") { sep = ',' }
    reader := csv.NewReader(strings.NewReader(raw)); reader.Comma = sep
    records, err := reader.ReadAll(); if err != nil || len(records) <= 1 { return err }
    headers := records[0]
    var wg sync.WaitGroup
    var mu sync.Mutex
    for i := 1; i < len(records); i++ {
       wg.Add(1)
       go func(idx int, row []string) {
          defer wg.Done()
          for j, cell := range row {
             if cell == "" { continue }
             colName := "Col"; if j < len(headers) { colName = headers[j] }

             // Ищем настройки для конкретной колонки
             settings, hasSettings := userReq.Columns[colName]
             if !hasSettings { continue }

             res := strings.TrimSpace(cell); changed := false

             // Логика ручного формата на основе мапы настроек
             if settings.Manual && settings.Format != "" {
                t := strings.ToLower(colName)
                if strings.Contains(t, "тел") || strings.Contains(settings.Format, "X") {
                    res = formatPhone(cell, settings.Format); changed = true
                } else if strings.Contains(t, "дат") || strings.Contains(settings.Format, "yyyy") {
                    res = formatDate(cell, settings.Format); changed = true
                } else if strings.Contains(t, "сумм") || strings.Contains(settings.Format, "234") {
                    res = formatNumber(cell, settings.Format); changed = true
                }
             }

             // Логика авто-правки, если ручной формат не сработал
             if !changed && settings.Auto {
                if cell != strings.TrimSpace(cell) {
                    res = strings.TrimSpace(cell)
                    changed = true
                }
             }

             if changed || cell != strings.TrimSpace(cell) {
                mu.Lock()
                meta.Errors = append(meta.Errors, models.FileError{Row: idx + 1, Column: colName, OldValue: cell, NewValue: res, Description: colName})
                if save { records[idx][j] = res }
                mu.Unlock()
             }
          }
       }(i, records[i])
    }
    wg.Wait()
    if save { f, _ := os.Create(meta.FilePath); defer f.Close(); w := csv.NewWriter(f); w.Comma = sep; w.WriteAll(records) }
    return nil
}