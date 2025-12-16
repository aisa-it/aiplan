package resources

import (
	"context"
	"errors"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"gorm.io/gorm"
)

type ResourceHandler func(db *gorm.DB, user *dao.User, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error)

type Resource struct {
	Resource mcp.Resource
	Handler  ResourceHandler
}

func WrapResource(db *gorm.DB, handler ResourceHandler) server.ResourceHandlerFunc {
	return func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		userRaw := ctx.Value("user")
		if userRaw == nil {
			return nil, errors.New("user not provided")
		}
		user := userRaw.(*dao.User)
		return handler(db, user, request)
	}
}
