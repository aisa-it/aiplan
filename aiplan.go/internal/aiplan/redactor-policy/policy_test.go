package policy

import (
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// TestUgcPolicyDocAnchors проверяет, что HTML якорей документа АИДока
// переживает санитизацию без потерь. Контракт фронта зафиксирован и меняться не может:
//   - явный якорь: <span class="doc-anchor" data-anchor-id="..." data-anchor-title="..."></span>
//   - заголовок с постоянным id: <h2 id="...">
//   - ссылки на якорь: <a href="#..."> и <a href="/ws/aidoc/uuid#...">
func TestUgcPolicyDocAnchors(t *testing.T) {
	cases := []struct {
		name     string
		in       string
		contains []string
	}{
		{
			name: "явный якорь сохраняет класс и оба data-атрибута",
			in:   `<p><span class="doc-anchor" data-anchor-id="vvedenie" data-anchor-title="Введение"></span>текст</p>`,
			contains: []string{
				`class="doc-anchor"`,
				`data-anchor-id="vvedenie"`,
				`data-anchor-title="Введение"`,
			},
		},
		{
			name:     "заголовок сохраняет id",
			in:       `<h2 id="vvedenie">Введение</h2>`,
			contains: []string{`<h2 id="vvedenie">`, `Введение`},
		},
		{
			name:     "id на всех уровнях заголовков",
			in:       `<h1 id="a1">a</h1><h3 id="b-2">b</h3><h6 id="c_3">c</h6>`,
			contains: []string{`<h1 id="a1">`, `<h3 id="b-2">`, `<h6 id="c_3">`},
		},
		{
			name:     "локальная ссылка на якорь",
			in:       `<p><a href="#vvedenie">текст</a></p>`,
			contains: []string{`href="#vvedenie"`, `текст`},
		},
		{
			name:     "ссылка на якорь в другом документе",
			in:       `<p><a href="/ws-slug/aidoc/2c0f6b1e-1111-2222-3333-444455556666#vvedenie">текст</a></p>`,
			contains: []string{`href="/ws-slug/aidoc/2c0f6b1e-1111-2222-3333-444455556666#vvedenie"`},
		},
		{
			name:     "id на span-якоре",
			in:       `<p><span id="vvedenie" class="doc-anchor" data-anchor-id="vvedenie"></span></p>`,
			contains: []string{`id="vvedenie"`, `class="doc-anchor"`},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := UgcPolicy.Sanitize(tc.in)
			for _, want := range tc.contains {
				if !strings.Contains(out, want) {
					t.Errorf("санитайзер потерял %q\nвход:  %s\nвыход: %s", want, tc.in, out)
				}
			}
		})
	}
}

// TestUgcPolicyDocAnchorsNegative — негативные кейсы: опасное содержимое
// не должно пролезать через атрибуты якорей.
func TestUgcPolicyDocAnchorsNegative(t *testing.T) {
	cases := []struct {
		name        string
		in          string
		notContains []string
	}{
		{
			name:        "обработчик события рядом с id вырезается",
			in:          `<h2 id="vvedenie" onclick="alert(1)">x</h2>`,
			notContains: []string{"onclick", "alert"},
		},
		{
			name:        "javascript-схема в href на якорь не проходит",
			in:          `<p><a href="javascript:alert(1)#vvedenie">x</a></p>`,
			notContains: []string{"javascript:", "<a "},
		},
		{
			name:        "скрипт внутри якоря вырезается",
			in:          `<span class="doc-anchor" data-anchor-id="a"><script>alert(1)</script></span>`,
			notContains: []string{"<script", "alert(1)"},
		},
		{
			name:        "id из не-ASCII символов не проходит",
			in:          `<h2 id="введение">x</h2>`,
			notContains: []string{"id="},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := UgcPolicy.Sanitize(tc.in)
			for _, bad := range tc.notContains {
				if strings.Contains(out, bad) {
					t.Errorf("санитайзер пропустил %q\nвход:  %s\nвыход: %s", bad, tc.in, out)
				}
			}
		})
	}
}

// TestUgcPolicyDocAnchorsNoAttrInjection проверяет, что кавычки внутри значений
// атрибутов якоря не разрывают атрибут и не превращаются в обработчик события.
// Подстрокой это не проверить: bluemonday оставляет значение экранированным
// (&#34;), поэтому "onmouseover" в выводе есть, но атрибутом не является.
// Считаем реальные атрибуты, разобрав вывод парсером.
func TestUgcPolicyDocAnchorsNoAttrInjection(t *testing.T) {
	cases := []string{
		`<span class="doc-anchor" data-anchor-id="a&#34; onmouseover=&#34;alert(1)"></span>`,
		`<span class="doc-anchor" data-anchor-title="a&#34; onmouseover=&#34;alert(1)"></span>`,
		`<h2 id="a&#34; onclick=&#34;alert(1)">x</h2>`,
	}

	for _, in := range cases {
		out := UgcPolicy.Sanitize(in)
		for _, attr := range collectAttrNames(t, out) {
			if strings.HasPrefix(attr, "on") {
				t.Errorf("после санитизации появился обработчик события %q\nвход:  %s\nвыход: %s", attr, in, out)
			}
		}
	}
}

// collectAttrNames разбирает HTML и возвращает имена всех атрибутов всех элементов.
func collectAttrNames(t *testing.T, htmlStr string) []string {
	t.Helper()

	root, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		t.Fatalf("не удалось разобрать вывод санитайзера: %v", err)
	}

	var names []string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			for _, a := range n.Attr {
				names = append(names, a.Key)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)

	return names
}

// TestUgcPolicyLinkNoFollow фиксирует побочный эффект UGCPolicy:
// RequireNoFollowOnLinks(true) дописывает rel="nofollow" ко всем ссылкам,
// включая якорные. Само значение href при этом не меняется.
func TestUgcPolicyLinkNoFollow(t *testing.T) {
	out := UgcPolicy.Sanitize(`<p><a href="#vvedenie">текст</a></p>`)
	if !strings.Contains(out, `rel="nofollow"`) {
		t.Errorf("ожидался rel=\"nofollow\", получено: %s", out)
	}
	if !strings.Contains(out, `href="#vvedenie"`) {
		t.Errorf("href якоря искажён: %s", out)
	}
}

// TestUgcPolicyIdRulesAreAdditive фиксирует ИЗВЕСТНОЕ поведение bluemonday:
// политики атрибутов складываются по ИЛИ (сначала правила элемента, потом
// глобальные — sanitize.go:512 и :527), а глобальное правило для id из
// AllowStandardAttributes использует неякорный regexp `[a-zA-Z0-9\:\-_\.]+`
// и матчится по подстроке. Поэтому добавленное в policy.go якорное правило
// формат id НЕ ужесточает: мусор с ASCII-символами внутри всё равно проходит.
//
// Тест намеренно закрепляет текущее (нежелательное, но безопасное — id не
// исполняемый атрибут) поведение. Если правило id когда-нибудь ужесточат
// переопределением глобальной политики, тест упадёт и приведёт сюда.
func TestUgcPolicyIdRulesAreAdditive(t *testing.T) {
	for _, in := range []string{
		`<h2 id="foo bar">x</h2>`,
		`<h2 id="javascript:alert(1)">x</h2>`,
		`<h2 id="ОченьДлинныйCamelCase">x</h2>`,
	} {
		out := UgcPolicy.Sanitize(in)
		if !strings.Contains(out, "id=") {
			t.Errorf("поведение id изменилось (стало строже) — обновите комментарий в policy.go\nвход:  %s\nвыход: %s", in, out)
		}
	}
}

// Проверяет, что правило формата data-anchor-id реально ограничивает значение,
// а не перекрывается более свободным правилом (политики складываются по ИЛИ).
func TestDocAnchorIdFormatEnforced(t *testing.T) {
	cases := []struct {
		name string
		in   string
		keep bool
	}{
		{"валидный слаг", `<span class="doc-anchor" data-anchor-id="vvedenie" data-anchor-title="Введение"></span>`, true},
		{"с цифрой и дефисом", `<span class="doc-anchor" data-anchor-id="razdel-2"></span>`, true},
		{"пробел внутри", `<span class="doc-anchor" data-anchor-id="foo bar"></span>`, false},
		{"верхний регистр", `<span class="doc-anchor" data-anchor-id="Vvedenie"></span>`, false},
		{"кириллица", `<span class="doc-anchor" data-anchor-id="введение"></span>`, false},
		{"начинается с дефиса", `<span class="doc-anchor" data-anchor-id="-foo"></span>`, false},
	}

	for _, c := range cases {
		got := UgcPolicy.Sanitize(c.in)
		has := strings.Contains(got, "data-anchor-id")
		if has != c.keep {
			t.Errorf("%s: ожидали keep=%v, получили %v\n  вход:  %s\n  выход: %s", c.name, c.keep, has, c.in, got)
		}
	}
}

// Человекочитаемое имя якоря форматом НЕ ограничено — там кириллица норма.
func TestDocAnchorTitleKeepsCyrillic(t *testing.T) {
	got := UgcPolicy.Sanitize(`<span class="doc-anchor" data-anchor-id="vvedenie" data-anchor-title="Введение в тему"></span>`)
	if !strings.Contains(got, "Введение в тему") {
		t.Errorf("название якоря потерялось: %s", got)
	}
}
