package resources

import (
	"encoding/json"
	"fmt"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"gorm.io/gorm"
)

var userResources = []Resource{
	{
		mcp.NewResource(
			"aiplan://users/current",
			"current_user_info",
			mcp.WithResourceDescription("Получение информации о текущем пользователе включая: имя, email, глобальные права, информацию о последней активности и настройки"),
			mcp.WithMIMEType("application/json"),
		),
		getCurrentUserInfo,
	},
}

func GetUsersResources(db *gorm.DB) []server.ServerResource {
	var resources []server.ServerResource
	for _, r := range userResources {
		resources = append(resources, server.ServerResource{
			Resource: r.Resource,
			Handler:  WrapResource(db, r.Handler),
		})
	}
	return resources
}

func getCurrentUserInfo(db *gorm.DB, user *dao.User, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	fmt.Println(request.Params)
	userJson, _ := json.Marshal(user.ToDTO())
	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      request.Params.URI,
			MIMEType: "application/json",
			Text:     string(userJson),
		},
	}, nil
}
