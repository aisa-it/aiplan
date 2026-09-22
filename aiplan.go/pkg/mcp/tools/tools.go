package tools

import (
	"context"
	"errors"

	"github.com/aisa-it/aiplan/aiplan.go/pkg/business"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/dao"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/policy"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/search"
	"github.com/gofrs/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"gorm.io/gorm"
)

var ErrInvalidArgType = errors.New("invalid mcp arg type")

// Deps — зависимости обработчиков. Передаются структурой, чтобы новая
// зависимость не меняла сигнатуру всех инструментов.
type Deps struct {
	DB *gorm.DB
	BL *business.Business
	// Policy — применитель правил движка, тот же, что и в HTTP.
	Policy *policy.Enforcer
	// Search — поиск задач с политикой видимости движка.
	Search *search.Searcher
}

// ToolHandler определяет сигнатуру функции-обработчика MCP инструмента.
type ToolHandler func(ctx context.Context, d Deps, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error)

// Tool представляет MCP инструмент с его обработчиком.
type Tool struct {
	Tool    mcp.Tool
	Handler ToolHandler
}

// WrapTool оборачивает обработчик инструмента, извлекая пользователя из контекста.
func WrapTool(d Deps, handler ToolHandler) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		userRaw := ctx.Value("user")
		if userRaw == nil {
			return nil, errors.New("user not provided")
		}
		user := userRaw.(*dao.User)
		return handler(ctx, d, user, request)
	}
}

// listResult оборачивает список в объект {count, result}.
// По спецификации MCP structuredContent обязан быть объектом: массив верхнего
// уровня клиенты с валидацией схемы (например, Claude Code) отбрасывают целиком.
// nil-слайс нормализуется в пустой, чтобы в JSON попал [], а не null.
func listResult[T any](items []T) (*mcp.CallToolResult, error) {
	if items == nil {
		items = make([]T, 0)
	}
	return mcp.NewToolResultJSON(map[string]any{
		"count":  len(items),
		"result": items,
	})
}

func GetUUIDArg(args map[string]any, argName string) (uuid.UUID, error) {
	raw, ok := args[argName]
	if !ok {
		return uuid.Nil, nil
	}
	rawStr, ok := raw.(string)
	if !ok {
		return uuid.Nil, ErrInvalidArgType
	}
	return uuid.FromString(rawStr)
}
