package service

import (
    "encoding/csv"
    "os"
    "regexp"
    "strings"
    "time"
    "doctordoc/internal/models"
)

func (s *fileService) processCSV(meta *models.FileMetadata, save bool) error {
    content, err := os.ReadFile(meta.FilePath)
    if err != nil {
       return err
    }

    raw := string(content)
    sep := ';'
    if strings.Count(raw, ",") > strings.Count(raw, ";") {
       sep = ','
    }

    reader := csv.NewReader(strings.NewReader(raw))
    reader.Comma = sep
    records, err := reader.ReadAll()
    if err != nil {
       return err
    }

    meta.Errors = []models.FileError{}
    if len(records) == 0 {
       return nil
    }
    headers := records[0]

    reEmail := regexp.MustCompile(`(?i)[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]{2,}`)
    reDate := regexp.MustCompile(`(\d{1,2})[./-]{1,2}(\d{1,2})[./-]{1,2}(\d{2,4})`)
    rePhone := regexp.MustCompile(`(\+7|7|8)?[\s\-]?\(?[9][0-9]{2}\)?[\s\-]?[0-9]{3}[\s\-]?[0-9]{2}[\s\-]?[0-9]{2}`)

    for i, row := range records {
       for j, cell := range row {
          if cell == "" {
             continue
          }

          trimmed := strings.TrimSpace(cell)
          currentResult := trimmed

          // Анализ Email
          currentResult = reEmail.ReplaceAllStringFunc(currentResult, func(m string) string {
             return strings.ToLower(strings.TrimSpace(m))
          })

          // Анализ Дат
          currentResult = reDate.ReplaceAllStringFunc(currentResult, func(m string) string {
             norm := regexp.MustCompile(`[/-]`).ReplaceAllString(m, ".")
             layouts := []string{"02.01.2006", "2.1.2006", "02.01.06", "2006.01.02"}
             for _, l := range layouts {
                if t, err := time.Parse(l, norm); err == nil {
                   return t.Format("02.01.2006")
                }
             }
             return m
          })

          // Анализ Телефонов
          currentResult = rePhone.ReplaceAllStringFunc(currentResult, func(m string) string {
             digits := regexp.MustCompile(`\D`).ReplaceAllString(m, "")
             if len(digits) >= 10 {
                return "7" + digits[len(digits)-10:]
             }
             return m
          })

          // Если есть изменения
          if currentResult != cell {
             col := "ПОЛЕ"
             if j < len(headers) {
                col = headers[j]
             }

             desc := "ФОРМАТ CSV"
             if strings.TrimSpace(cell) != cell {
                desc = "ПРОБЕЛЫ"
             }

             meta.Errors = append(meta.Errors, s.createError(i+1, col, cell, currentResult, desc))
             if save {
                records[i][j] = currentResult
             }
          }
       }
    }

    meta.RowsCount = len(records)
    if save {
       f2, _ := os.Create(meta.FilePath)
       defer f2.Close()
       w := csv.NewWriter(f2)
       w.Comma = sep
       w.WriteAll(records)
    }
    return nil
}