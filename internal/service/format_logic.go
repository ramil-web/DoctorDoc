package service

import (
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func formatDate(val, customMask string) string {
	if val == "" {
		return ""
	}

	norm := regexp.MustCompile(`[/-]`).ReplaceAllString(val, ".")
	norm = strings.ReplaceAll(norm, ",", ".")

	layouts := []string{
		"02.01.2006", "02.01.06", "2006.01.02", "01.02.2006",
		"02.01.2006 15:04", "2006-01-02", "2.1.2006",
	}

	var t time.Time
	found := false
	for _, l := range layouts {
		if parsed, err := time.Parse(l, norm); err == nil {
			t = parsed
			found = true
			break
		}
	}

	if !found {
		log.Printf("[GO/Date] ❌ Could not parse date: %s", val)
		return val
	}

	f := customMask
	if f == "" {
		f = "02.01.2006"
	}

	f = strings.ReplaceAll(f, "yyyy", "2006")
	f = strings.ReplaceAll(f, "yy", "06")
	f = strings.ReplaceAll(f, "MM", "01")
	f = strings.ReplaceAll(f, "dd", "02")
	f = strings.ReplaceAll(f, "HH", "15")
	f = strings.ReplaceAll(f, "mm", "04")
	f = strings.ReplaceAll(f, "ss", "05")

	res := t.Format(f)
	log.Printf("[GO/Date] ✅ Formatted %s -> %s using %s", val, res, f)
	return res
}

func formatNumber(val, mask string) string {
	if val == "" {
		return ""
	}
	clean := regexp.MustCompile(`[^0-9,.-]`).ReplaceAllString(val, "")
	clean = strings.ReplaceAll(clean, ",", ".")
	num, err := strconv.ParseFloat(clean, 64)
	if err != nil {
		log.Printf("[GO/Num] ❌ Could not parse number: %s", val)
		return val
	}

	var result string
	if mask == "int" {
		result = fmt.Sprintf("%.0f", num)
	} else {
		thousandSep := ""
		if strings.Contains(mask, "1,234") { thousandSep = "," }
		if strings.Contains(mask, "1 234") { thousandSep = " " }
		decimalSep := "."
		if strings.Contains(mask, "34,56") { decimalSep = "," }

		res := fmt.Sprintf("%.2f", num)
		parts := strings.Split(res, ".")
		intP, decP := parts[0], parts[1]

		var withSep string
		for i, v := range intP {
			if i > 0 && (len(intP)-i)%3 == 0 && thousandSep != "" {
				withSep += thousandSep
			}
			withSep += string(v)
		}
		result = withSep + decimalSep + decP
	}
	log.Printf("[GO/Num] ✅ Formatted %s -> %s (Mask: %s)", val, result, mask)
	return result
}

func formatPhone(val, mask string) string {
	if val == "" {
		return ""
	}

	// 1. Извлекаем только цифры из входного значения
	re := regexp.MustCompile(`\D`)
	digits := re.ReplaceAllString(val, "")

	if len(digits) < 10 {
		return val
	}

	// 2. Нам нужны последние 10 цифр самого номера (без 7 или 8 в начале)
	pure := digits[len(digits)-10:]

	// 3. Если маска не содержит 'X', значит это просто требование "слитного" формата
	if !strings.Contains(mask, "X") {
		// Если в маске указано 8XXXXXXXXXX или +7... но без иксов,
		// определяем префикс по маске
		prefix := "7"
		if strings.HasPrefix(mask, "8") {
			prefix = "8"
		}
		res := prefix + pure
		log.Printf("[GO/Phone] ✅ Simple Format: %s -> %s", val, res)
		return res
	}

	// 4. Применяем маску посимвольно
	// Мы идем по маске и заменяем каждый 'X' на цифру из наших 10 чистых цифр
	result := ""
	digitIdx := 0
	for _, char := range mask {
		if char == 'X' {
			if digitIdx < len(pure) {
				result += string(pure[digitIdx])
				digitIdx++
			}
		} else {
			// Все остальные символы (+, пробелы, скобки, тире, цифры 7 или 8)
			// переносим как есть из маски
			result += string(char)
		}
	}

	log.Printf("[GO/Phone] ✅ Mask Format: %s -> %s (Mask: %s)", val, result, mask)
	return result
}