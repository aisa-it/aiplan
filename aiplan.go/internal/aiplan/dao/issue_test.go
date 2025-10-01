// Содержит функции для санитайзации HTML-текста в соответствии с политиками безопасности.
//
// Основные возможности:
//   - Санитайзация текста с использованием предопределенных политик.
//   - Применение различных политик к разным типам HTML-контента (mark, mention, table, img).
package dao

import (
	"fmt"
	"testing"

	policy "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/redactor-policy"
)

// TestHTMLSanitizer проверяет корректность работы функции Sanitize для различных типов HTML-контента.
// Функция выполняет санитайзацию HTML-текста с использованием предопределенных политик безопасности.
// Она принимает тестировщик и не возвращает значения, а лишь выводит результаты санитайзации в консоль.
//
// Параметры:
//   - t: тестировщик для выполнения тестов.
//
// Возвращает:
//   - Нет (void). Результаты санитайзации выводятся в консоль с помощью fmt.Printf.
func TestHTMLSanitizer(t *testing.T) {
	var markHTML, mentionHTML, tableHTML, imgHTML string
	{
		markHTML = `<p>
  <span style="color: rgb(245,0,0)">
  <mark data-color="rgb(255,255,0)" style="background-color: rgb(255,255,0); color: inherit">asdas</mark>
  </span>
</p>`

		mentionHTML = `<p>
  <span
  class="mention"
  data-type="mention"
  data-id="Grigoriy"
  data-label="{&quot;avatar&quot;:&quot;&quot;,&quot;username&quot;:&quot;Grigoriy&quot;,&quot;email&quot;:&quot;grigory.vasiliev@aisa.ru&quot;,&quot;avatarText&quot;:&quot;В Г&quot;,&quot;title&quot;:&quot;Васильев Григорий&quot;}"
>
  @Grigoriy
  </span>
</p>`

		tableHTML = ` <table style="width: 358px">
  <colgroup>
  <col style="width: 100px">
  <col style="width: 100px">
  <col style="width: 158px">
  </colgroup>
  <tbody>
  <tr>
    <th colspan="1" rowspan="1" colwidth="100">
    <p>Таблица</p></th>
    <th colspan="1" rowspan="1" colwidth="100"><p></p></th>
    <th colspan="1" rowspan="1" colwidth="158"><p></p></th>
  </tr>
  <tr>
    <td colspan="1" rowspan="1" colwidth="100">
    <p></p>
    </td>
    <td colspan="1" rowspan="1" colwidth="100">
    <p></p>
    </td>
    <td colspan="1" rowspan="1" colwidth="158">
    <p></p>
    </td>
  </tr>
  <tr>
      <td colspan="1" rowspan="1" colwidth="100">
    <p></p>
    </td>
    <td colspan="2" rowspan="1" colwidth="100,158">
    <p>Объединение</p>
    </td>
  </tr>
  </tbody>
</table>`

		imgHTML = `<p><img src="/uploads/9737d98d-21c6-4283-87f8-593f1f467874/ecbe6b53b61048ecac3df27b575f799b-image.jpg" alt="image_2024-08-09_18-56-30" style="display: inline-block; max-width: 427px; height: auto; float: right; margin: ; width: 427px;" width="427" draggable="true"></p>`
	}

	fmt.Printf("Mark: %s\n\n", policy.UgcPolicy.Sanitize(markHTML))
	fmt.Printf("Mention: %s\n\n", policy.UgcPolicy.Sanitize(mentionHTML))
	fmt.Printf("Table: %s\n\n", policy.UgcPolicy.Sanitize(tableHTML))
	fmt.Printf("Img: %s\n\n", policy.UgcPolicy.Sanitize(imgHTML))
}
