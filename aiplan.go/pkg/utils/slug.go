package utils

import (
	"strings"
	"unicode"

	"github.com/gofrs/uuid"
)

// Таблица повторяет cyrillic-to-translit-js (пресет ru), которым фронт строит
// слаги пространств и якорей: в начале слова е→ye, й→y, дальше е→e, й→i.
var translitFirst = map[rune]string{'е': "ye", 'й': "y"}

var translitAny = map[rune]string{
	'а': "a", 'б': "b", 'в': "v", 'г': "g", 'д': "d", 'е': "e", 'ё': "yo", 'ж': "zh",
	'з': "z", 'и': "i", 'й': "i", 'к': "k", 'л': "l", 'м': "m", 'н': "n", 'о': "o",
	'п': "p", 'р': "r", 'с': "s", 'т': "t", 'у': "u", 'ф': "f", 'х': "kh", 'ц': "ts",
	'ч': "ch", 'ш': "sh", 'щ': "shch", 'ъ': "", 'ы': "i", 'ь': "", 'э': "e", 'ю': "yu", 'я': "ya",
}

const (
	slugMaxLen   = 100
	slugFallback = "doc"
)

// Translit переводит кириллицу в латиницу, остальные символы не трогает.
func Translit(s string) string {
	var b strings.Builder
	wordStart := true
	for _, r := range s {
		if unicode.IsSpace(r) {
			b.WriteRune(r)
			wordStart = true
			continue
		}
		lower := unicode.ToLower(r)
		out, ok := "", false
		if wordStart {
			out, ok = translitFirst[lower]
		}
		if !ok {
			out, ok = translitAny[lower]
		}
		wordStart = false
		if !ok {
			b.WriteRune(r)
			continue
		}
		if unicode.IsUpper(r) && out != "" {
			out = strings.ToUpper(out[:1]) + out[1:]
		}
		b.WriteString(out)
	}
	return b.String()
}

// Slugify делает из названия фрагмент адреса: транслит, строчные буквы,
// всё кроме [a-z0-9] схлопывается в дефис.
func Slugify(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(Translit(s)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
			continue
		}
		if b.Len() > 0 && !dash {
			b.WriteByte('-')
			dash = true
		}
	}
	slug := b.String()
	if len(slug) > slugMaxLen {
		slug = slug[:slugMaxLen]
	}
	slug = strings.TrimRight(slug, "-")
	if slug == "" {
		return slugFallback
	}
	// UUID в адресе означает документ по id, слаг не должен с ним путаться.
	if _, err := uuid.FromString(slug); err == nil {
		slug += "-" + slugFallback
	}
	return slug
}
