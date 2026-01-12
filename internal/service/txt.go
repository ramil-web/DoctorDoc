package service

import (
    "doctordoc/internal/models"
    "os"
    "regexp"
    "strings"
)

func (s *fileService) processTXT(meta *models.FileMetadata, save bool) error {
    content, err := os.ReadFile(meta.FilePath)
    if err != nil {
       return err
    }

    lines := strings.Split(string(content), "\n")
    meta.Errors = []models.FileError{}

    reEmail := regexp.MustCompile(`(?i)[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]{2,}`)

    for i, line := range lines {
       if line == "" {
          continue
       }

       trimmed := strings.TrimSpace(line)
       currentResult := reEmail.ReplaceAllStringFunc(trimmed, func(m string) string {
          return strings.ToLower(strings.TrimSpace(m))
       })

       if currentResult != line {
          meta.Errors = append(meta.Errors, s.createError(i+1, "СТРОКА", line, currentResult, "ФИКС ТЕКСТА"))
          if save {
             lines[i] = currentResult
          }
       }
    }

    meta.RowsCount = len(lines)
    if save {
       return os.WriteFile(meta.FilePath, []byte(strings.Join(lines, "\n")), 0644)
    }
    return nil
}