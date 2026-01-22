package service

import (
	"doctordoc/internal/models"
	"os"
	"regexp"
	"strings"
	"time"
)

func (s *fileService) processTXT(meta *models.FileMetadata, save bool) error {
	content, err := os.ReadFile(meta.FilePath)
	if err != nil {
		return err
	}

	rawContent := string(content)
	rawContent = strings.ReplaceAll(rawContent, "\r\n", "\n")
	lines := strings.Split(rawContent, "\n")

	meta.Errors = []models.FileError{}
	meta.RowsCount = len(lines)

	reEmail := regexp.MustCompile(`(?i)[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]{2,}`)
	rePhone := regexp.MustCompile(`(\+7|7|8)?[\s\-]?\(?[9][0-9]{2}\)?[\s\-]?[0-9]{3}[\s\-]?[0-9]{2}[\s\-]?[0-9]{2}`)
	reSimpleDate := regexp.MustCompile(`(\d{2,4})[/-](\d{1,2})[/-](\d{2,4})`)

	for i, line := range lines {
		if line == "" { continue }

		trimmedLine := strings.TrimSpace(line)
		currentResult := trimmedLine
		foundError := false

		// 1. Почта
		currentResult = reEmail.ReplaceAllStringFunc(currentResult, func(m string) string {
			res := strings.ToLower(m)
			if res != m { foundError = true }
			return res
		})

		// 2. Телефоны
		currentResult = rePhone.ReplaceAllStringFunc(currentResult, func(m string) string {
			digits := regexp.MustCompile(`\D`).ReplaceAllString(m, "")
			if len(digits) >= 10 {
				res := "7" + digits[len(digits)-10:]
				if res != m { foundError = true }
				return res
			}
			return m
		})

		// 3. Даты
		currentResult = reSimpleDate.ReplaceAllStringFunc(currentResult, func(m string) string {
			if strings.Contains(m, ":") { return m }
			norm := regexp.MustCompile(`[/-]`).ReplaceAllString(m, ".")
			layouts := []string{"02.01.2006", "2006.01.02", "02.01.06"}
			for _, l := range layouts {
				if t, err := time.Parse(l, norm); err == nil {
					res := t.Format("02.01.2006")
					if res != m { foundError = true }
					return res
				}
			}
			return m
		})

		// 4. Пробелы (сравнение оригинала и trimmed)
		isTrimmed := (line != trimmedLine)

		if foundError || isTrimmed {
			meta.Errors = append(meta.Errors, models.FileError{
				Row:         i + 1,
				Column:      "СТРОКА",
				OldValue:    `"` + line + `"`,
				NewValue:    `"` + currentResult + `"`,
				Description: "ТЕКСТОВЫЕ ДАННЫЕ",
			})

			if save {
				lines[i] = currentResult
			}
		}
	}

	if save {
		return os.WriteFile(meta.FilePath, []byte(strings.Join(lines, "\n")), 0644)
	}
	return nil
}