// Пакет defaultengine — движок ядра, воспроизводящий штатное поведение
// трекера: права по ролям участника, переходы статусов по states_flow,
// видимость задач по членству в проектах и Lua-хуки проекта.
//
// Реализации правил переносятся сюда из обработчиков постепенно: пока
// какой-то интерфейс не реализован, ядро использует прежний код.
package defaultengine

import (
	"context"

	"github.com/aisa-it/aiplan/aiplan.go/pkg/engine"
)

// Engine — движок по умолчанию.
type Engine struct {
	core engine.Core
}

// New создаёт движок по умолчанию.
func New() *Engine { return &Engine{} }

// Name возвращает имя движка.
func (e *Engine) Name() string { return "default" }

// Init сохраняет ссылку на ядро. Дополнительных моделей, роутов и
// периодических задач движок не добавляет.
func (e *Engine) Init(_ context.Context, core engine.Core) error {
	e.core = core
	return nil
}

// Core возвращает ядро, полученное при инициализации.
func (e *Engine) Core() engine.Core { return e.core }

// Проверка соответствия контракту на этапе компиляции.
var _ engine.Engine = (*Engine)(nil)
