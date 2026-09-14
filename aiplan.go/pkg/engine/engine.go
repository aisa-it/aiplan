// Пакет engine описывает контракт подключаемого движка — набора правил,
// определяющих поведение трекера: кто что может делать с задачей, какие
// переходы статусов допустимы и какие задачи пользователь видит.
//
// Ядро содержит дефолтный движок (пакет defaultengine), воспроизводящий
// штатное поведение. Сборка под конкретного заказчика подключает свой,
// не меняя ядро.
//
// Обязателен только интерфейс Engine. Остальные интерфейсы пакета —
// опциональные: ядро находит их через приведение типа, а для нереализованных
// берёт дефолтное поведение. Благодаря этому новые точки расширения
// добавляются без слома существующих движков.
package engine

import "context"

// Engine — подключаемая реализация поведения трекера.
// Init вызывается один раз до старта HTTP-сервера.
type Engine interface {
	// Name — имя движка для логов и диагностики.
	Name() string
	// Init получает доступ к ядру и регистрирует расширения.
	// Вызывается до регистрации роутов и до миграции схемы.
	Init(ctx context.Context, core Core) error
}

// Decision — решение движка по запросу.
type Decision uint8

const (
	// DecisionDefault — движок не имеет мнения, решает ядро.
	DecisionDefault Decision = iota
	DecisionAllow
	DecisionDeny
)

// Verdict — ответ движка на проверку права или перехода.
type Verdict struct {
	Decision Decision
	// Error — что вернуть клиенту при DecisionDeny.
	// nil — ядро подставит ошибку по умолчанию.
	Error *DefinedError
}

// Allow, Deny, Default — готовые вердикты для частых случаев.
var (
	Allow   = Verdict{Decision: DecisionAllow}
	Deny    = Verdict{Decision: DecisionDeny}
	Default = Verdict{}
)

// Allowed сообщает, разрешает ли вердикт действие при заданном
// поведении по умолчанию.
func (v Verdict) Allowed(byDefault bool) bool {
	switch v.Decision {
	case DecisionAllow:
		return true
	case DecisionDeny:
		return false
	default:
		return byDefault
	}
}
