package engine

import "github.com/aisa-it/aiplan/aiplan.go/pkg/apierrors"

// DefinedError — ошибка ядра с кодом и локализованным текстом.
// Псевдоним, чтобы движок не зависел от расположения пакета ошибок
// и мог возвращать как готовые ошибки ядра, так и свои.
type DefinedError = apierrors.DefinedError

// Ошибки, подставляемые ядром, когда движок вернул Deny без своей ошибки.
var (
	ErrForbidden      = apierrors.ErrIssueForbidden
	ErrForbiddenState = apierrors.ErrForbiddenState
)
