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

	// Нормализуем строки
	rawContent := string(content)
	rawContent = strings.ReplaceAll(rawContent, "\r\n", "\n")
	lines := strings.Split(rawContent, "\n")

	meta.Errors = []models.FileError{}
	meta.RowsCount = len(lines)

	// Регулярки для поиска и замены ВНУТРИ текста
	reEmail := regexp.MustCompile(`(?i)[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]{2,}`)
	reDate := regexp.MustCompile(`(\d{1,2})[./-]{1,2}(\d{1,2})[./-]{1,2}(\d{2,4})`)
	rePhone := regexp.MustCompile(`(\+7|7|8)?[\s\-]?\(?[9][0-9]{2}\)?[\s\-]?[0-9]{3}[\s\-]?[0-9]{2}[\s\-]?[0-9]{2}`)
	reDigitsOnly := regexp.MustCompile(`\D`)

	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}

		// Берем всю строку как базу для исправлений
		currentResult := line

		// 1. Ищем и исправляем все Email в строке
		currentResult = reEmail.ReplaceAllStringFunc(currentResult, func(m string) string {
			return strings.ToLower(strings.TrimSpace(m))
		})

		// 2. Ищем и исправляем все Телефоны в строке
		currentResult = rePhone.ReplaceAllStringFunc(currentResult, func(m string) string {
			digits := reDigitsOnly.ReplaceAllString(m, "")
			if len(digits) >= 10 {
				return "7" + digits[len(digits)-10:]
			}
			return m
		})

		// 3. Ищем и исправляем все Даты в строке
		currentResult = reDate.ReplaceAllStringFunc(currentResult, func(m string) string {
			norm := regexp.MustCompile(`[/-]`).ReplaceAllString(m, ".")
			norm = regexp.MustCompile(`\.{2,}`).ReplaceAllString(norm, ".")
			layouts := []string{"02.01.2006", "2.1.2006", "02.01.06", "2006.01.02"}
			for _, l := range layouts {
				if t, err := time.Parse(l, norm); err == nil {
					return t.Format("02.01.2006")
				}
			}
			return m
		})

		// Если в строке нашлось хоть одно изменение (почта, телефон или дата)
		if currentResult != line {
			meta.Errors = append(meta.Errors, models.FileError{
				Row:         i + 1,
				Column:      "СТРОКА", // Показываем, что это целая строка
				OldValue:    line,      // Вся старая строка со всеми данными
				NewValue:    currentResult, // Вся исправленная строка
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