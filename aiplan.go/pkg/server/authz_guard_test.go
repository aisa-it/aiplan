package server

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestNoHandmadePermissionChecks следит, чтобы проверки прав не заводились
// в обработчиках задач заново.
//
// Права решает движок: и до загрузки объекта (middleware роута), и после
// (policy.Authorize с policy.On). Проверка, написанная рядом с обработчиком
// вручную, движку не видна — заказной движок её не переопределит, и права
// разъедутся между тем, что объявлено, и тем, что выполняется.
//
// Список исключений — то, что ещё не перенесено. Он должен сокращаться,
// а не расти.
func TestNoHandmadePermissionChecks(t *testing.T) {
	// Признаки решения о правах: сравнение ролей и авторства.
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`Role\s*[!=]=\s*types\.\w+Role`),
		regexp.MustCompile(`CreatedById\s*[!=]=\s*user\.ID`),
		regexp.MustCompile(`ActorId\.UUID\s*[!=]=\s*user\.ID`),
	}

	// Известные исключения: правила, ещё не перенесённые в движок.
	// Список должен сокращаться, а не расти.
	knownExceptions := map[string]string{
		// Переход статуса: StatePolicy движка — предмет отдельного этапа.
		"ErrForbiddenState": "states_flow",
	}

	// Ещё не перенесено: права на проект, пространство, спринт, документы
	// и формы решают отдельные функции по пути роута.
	allowed := map[string]struct{}{
		"http-project.go":   {},
		"http-workspace.go": {},
		"http-sprint.go":    {},
		"http-doc.go":       {},
		"http-form.go":      {},
		"http-user.go":      {},
		"http-admin.go":     {},
		"http-backup.go":    {},
		"http-git.go":       {},
		"http-import.go":    {},
	}

	files, err := filepath.Glob("http-issue*.go")
	if err != nil {
		t.Fatalf("поиск обработчиков: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("обработчики задач не найдены — проверка сломана")
	}

	for _, f := range files {
		if _, ok := allowed[filepath.Base(f)]; ok {
			continue
		}

		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("чтение %s: %v", f, err)
		}

		lines := strings.Split(string(src), "\n")
		for i, line := range lines {
			if strings.HasPrefix(strings.TrimSpace(line), "//") {
				continue
			}
			if isKnownException(lines, i, knownExceptions) {
				continue
			}
			if !deniesNearby(lines, i) {
				// Сравнение роли или авторства само по себе не является
				// решением о правах: так же выглядят админский обход
				// бизнес-правил и подготовка данных для ответа.
				continue
			}
			for _, p := range patterns {
				if p.MatchString(line) {
					t.Errorf("%s:%d: проверка прав в обработчике — перенесите в движок "+
						"(policy.Authorize с policy.On для правил по объекту):\n\t%s",
						f, i+1, strings.TrimSpace(line))
				}
			}
		}
	}
}

// deniesNearby сообщает, что рядом со строкой возвращается отказ в доступе:
// именно это отличает решение о правах от прочих сравнений роли.
func deniesNearby(lines []string, i int) bool {
	const window = 4
	from := max(0, i-1)
	to := min(len(lines), i+window)

	for _, l := range lines[from:to] {
		if strings.Contains(l, "Forbidden") {
			return true
		}
	}
	return false
}

// isKnownException сообщает, что рядом со строкой возвращается ошибка
// из списка ещё не перенесённых правил.
func isKnownException(lines []string, i int, exceptions map[string]string) bool {
	const window = 6
	from := max(0, i-1)
	to := min(len(lines), i+window)

	for _, l := range lines[from:to] {
		for errName := range exceptions {
			if strings.Contains(l, errName) {
				return true
			}
		}
	}
	return false
}
