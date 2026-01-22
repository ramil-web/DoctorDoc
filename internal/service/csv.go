package service

import (
	"doctordoc/internal/models"
	"encoding/csv"
	"os"
	"regexp"
	"strings"
	"time"
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
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil {
		return err
	}

	meta.RowsCount = len(records)
	meta.Errors = []models.FileError{}
	if len(records) == 0 {
		return nil
	}
	headers := records[0]

	reEmail := regexp.MustCompile(`(?i)[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]{2,}`)
	rePhone := regexp.MustCompile(`(\+7|7|8)?[\s\-]?\(?[9][0-9]{2}\)?[\s\-]?[0-9]{3}[\s\-]?[0-9]{2}[\s\-]?[0-9]{2}`)
	reSimpleDate := regexp.MustCompile(`^(\d{2,4})[/-](\d{1,2})[/-](\d{2,4})$`)

	for i, row := range records {
		if i == 0 { continue }

		for j, cell := range row {
			if cell == "" { continue }

			colName := "НЕИЗВЕСТНО"
			if j < len(headers) && headers[j] != "" {
				colName = headers[j]
			}

			trimmed := strings.TrimSpace(cell)
			currentResult := trimmed
			foundError := false

			// 1. ПОЧТА
			if strings.Contains(trimmed, "@") && strings.Contains(trimmed, " ") {
				noSpaces := strings.ReplaceAll(trimmed, " ", "")
				if reEmail.MatchString(noSpaces) {
					currentResult = strings.ToLower(noSpaces)
					foundError = true
				}
			}

			// 2. ТЕЛЕФОН
			if !foundError && rePhone.MatchString(trimmed) {
				digits := regexp.MustCompile(`\D`).ReplaceAllString(trimmed, "")
				if len(digits) >= 10 {
					formatted := "7" + digits[len(digits)-10:]
					if formatted != cell {
						currentResult = formatted
						foundError = true
					}
				}
			}

			// 3. ДАТА
			if !foundError && !strings.Contains(trimmed, ":") && reSimpleDate.MatchString(trimmed) {
				norm := regexp.MustCompile(`[/-]`).ReplaceAllString(trimmed, ".")
				layouts := []string{"02.01.2006", "2006.01.02", "02.01.06"}
				for _, l := range layouts {
					if t, err := time.Parse(l, norm); err == nil {
						currentResult = t.Format("02.01.2006")
						if currentResult != cell {
							foundError = true
						}
						break
					}
				}
			}

			// 4. ПРОБЕЛЫ ПО КРАЯМ
			if !foundError && cell != trimmed {
				currentResult = trimmed
				foundError = true
			}

			// РЕГИСТРАЦИЯ
			if foundError && currentResult != cell {
				meta.Errors = append(meta.Errors, models.FileError{
					Row:         i + 1,
					Column:      colName,
					OldValue:    `"` + cell + `"`,
					NewValue:    `"` + currentResult + `"`,
					Description: colName,
				})

				if save {
					records[i][j] = currentResult
				}
			}
		}
	}

	if save {
		f2, err := os.Create(meta.FilePath)
		if err != nil {
			return err
		}
		defer f2.Close()

		w := csv.NewWriter(f2)
		w.Comma = sep
		if err := w.WriteAll(records); err != nil {
			return err
		}
		w.Flush()
	}
	return nil
}