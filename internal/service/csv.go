package service

import (
	"doctordoc/internal/models"
	"encoding/csv"
	"os"
	"regexp"
	"strconv"
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
	// Позволяем переменное количество полей, чтобы не падать на кривых строках
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

	// РЕГУЛЯРНЫЕ ВЫРАЖЕНИЯ (Один в один как в XLSX)
	reEmail := regexp.MustCompile(`(?i)[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]{2,}`)
	reDate := regexp.MustCompile(`(\d{1,2})[./-]{1,2}(\d{1,2})[./-]{1,2}(\d{2,4})`)
	rePhone := regexp.MustCompile(`(\+7|7|8)?[\s\-]?\(?[9][0-9]{2}\)?[\s\-]?[0-9]{3}[\s\-]?[0-9]{2}[\s\-]?[0-9]{2}`)
	reINN := regexp.MustCompile(`^\d{10}$|^\d{12}$`)
	reBIK := regexp.MustCompile(`^04\d{7}$`)
	reOGRN := regexp.MustCompile(`^\d{13}$|^\d{15}$`)
	reNonDigits := regexp.MustCompile(`\D`)

	for i, row := range records {
		// Пропускаем заголовок точно так же, как в XLSX
		if i == 0 {
			continue
		}

		for j, cell := range row {
			if strings.TrimSpace(cell) == "" {
				continue
			}

			// Определяем имя колонки из заголовков
			colName := "НЕИЗВЕСТНО"
			if j < len(headers) && headers[j] != "" {
				colName = headers[j]
			}

			trimmed := strings.TrimSpace(cell)
			currentResult := trimmed
			foundError := false

			// 1. ПРОВЕРКА НА ПОЧТУ (с удалением пробелов внутри)
			if strings.Contains(trimmed, "@") {
				noSpaces := strings.ReplaceAll(trimmed, " ", "")
				found := reEmail.FindString(noSpaces)
				if found != "" {
					currentResult = strings.ToLower(found)
					foundError = true
				}
			}

			// 2. ПРОВЕРКА НА ТЕЛЕФОН
			if !foundError {
				if rePhone.MatchString(trimmed) {
					digits := reNonDigits.ReplaceAllString(trimmed, "")
					if len(digits) >= 10 {
						currentResult = "7" + digits[len(digits)-10:]
						foundError = true
					}
				}
			}

			// 3. ПРОВЕРКА НА ДАТУ
			if !foundError {
				if reDate.MatchString(trimmed) {
					norm := regexp.MustCompile(`[/-]`).ReplaceAllString(trimmed, ".")
					norm = regexp.MustCompile(`\.{2,}`).ReplaceAllString(norm, ".")
					layouts := []string{"02.01.2006", "2.1.2006", "02.01.06", "2006.01.02"}
					for _, l := range layouts {
						if t, err := time.Parse(l, norm); err == nil {
							currentResult = t.Format("02.01.2006")
							foundError = true
							break
						}
					}
				}
			}

			// 4. ПРОВЕРКА НА СУММУ
			if !foundError {
				cleanSum := strings.ReplaceAll(trimmed, ",", ".")
				cleanSum = regexp.MustCompile(`[^\d.]`).ReplaceAllString(cleanSum, "")
				if _, err := strconv.ParseFloat(cleanSum, 64); err == nil && cleanSum != "" {
					if cleanSum != cell {
						currentResult = cleanSum
						foundError = true
					}
				}
			}

			// 5. ИНН / БИК / ОГРН
			if !foundError {
				onlyDigits := reNonDigits.ReplaceAllString(trimmed, "")
				if reINN.MatchString(onlyDigits) || reBIK.MatchString(onlyDigits) || reOGRN.MatchString(onlyDigits) {
					currentResult = onlyDigits
					foundError = true
				}
			}

			// 6. ЛИШНИЕ ПРОБЕЛЫ
			if !foundError && trimmed != cell {
				currentResult = trimmed
				foundError = true
			}

			// ФИНАЛЬНЫЙ СБОР ОШИБОК (как в XLSX)
			if currentResult != cell && foundError {
				meta.Errors = append(meta.Errors, models.FileError{
					Row:         i + 1,
					Column:      colName,
					OldValue:    cell,
					NewValue:    currentResult,
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
		// Важно: Flush гарантирует запись данных из буфера в файл
		if err := w.WriteAll(records); err != nil {
			return err
		}
		w.Flush()
	}
	return nil
}