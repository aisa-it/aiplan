package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/business"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dto"
	"github.com/gofrs/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// setupTestDB создает тестовую БД SQLite в памяти с упрощёнными таблицами
func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	// Создаем упрощённые таблицы вручную (без PostgreSQL-специфичных индексов)
	err = db.Exec(`
		CREATE TABLE users (
			id TEXT PRIMARY KEY
		)
	`).Error
	require.NoError(t, err)

	err = db.Exec(`
		CREATE TABLE workspaces (
			id TEXT PRIMARY KEY,
			slug TEXT,
			name TEXT
		)
	`).Error
	require.NoError(t, err)

	err = db.Exec(`
		CREATE TABLE projects (
			id TEXT PRIMARY KEY,
			workspace_id TEXT,
			name TEXT,
			identifier TEXT
		)
	`).Error
	require.NoError(t, err)

	err = db.Exec(`
		CREATE TABLE project_members (
			id TEXT PRIMARY KEY,
			member_id TEXT,
			project_id TEXT,
			role INTEGER
		)
	`).Error
	require.NoError(t, err)

	err = db.Exec(`
		CREATE TABLE states (
			id TEXT PRIMARY KEY,
			name TEXT,
			description TEXT,
			color TEXT,
			slug TEXT,
			sequence INTEGER,
			"group" TEXT,
			"default" INTEGER DEFAULT 0,
			project_id TEXT,
			workspace_id TEXT
		)
	`).Error
	require.NoError(t, err)

	return db
}

// Хелперы для вставки тестовых данных через raw SQL
func insertUser(t *testing.T, db *gorm.DB, id uuid.UUID) {
	err := db.Exec(`INSERT INTO users (id) VALUES (?)`, id.String()).Error
	require.NoError(t, err)
}

func insertWorkspace(t *testing.T, db *gorm.DB, id uuid.UUID, slug string) {
	err := db.Exec(`INSERT INTO workspaces (id, slug) VALUES (?, ?)`, id.String(), slug).Error
	require.NoError(t, err)
}

func insertProject(t *testing.T, db *gorm.DB, id, workspaceId uuid.UUID) {
	err := db.Exec(`INSERT INTO projects (id, workspace_id) VALUES (?, ?)`, id.String(), workspaceId.String()).Error
	require.NoError(t, err)
}

func insertProjectMember(t *testing.T, db *gorm.DB, id, memberId, projectId uuid.UUID, role int) {
	err := db.Exec(`INSERT INTO project_members (id, member_id, project_id, role) VALUES (?, ?, ?, ?)`,
		id.String(), memberId.String(), projectId.String(), role).Error
	require.NoError(t, err)
}

func insertState(t *testing.T, db *gorm.DB, id uuid.UUID, name, color, group string, sequence int, projectId, workspaceId uuid.UUID) {
	err := db.Exec(`INSERT INTO states (id, name, color, "group", sequence, project_id, workspace_id) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id.String(), name, color, group, sequence, projectId.String(), workspaceId.String()).Error
	require.NoError(t, err)
}

// createTestRequest создает MCP CallToolRequest с заданными аргументами
func createTestRequest(args map[string]interface{}) mcp.CallToolRequest {
	return mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: args,
		},
	}
}

func TestProjectPermissionsMiddleware(t *testing.T) {
	t.Run("отсутствие project_id возвращает ошибку", func(t *testing.T) {
		db := setupTestDB(t)
		user := &dao.User{ID: uuid.Must(uuid.NewV4())}

		handler := func(ctx context.Context, db *gorm.DB, bl *business.Business, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			t.Fatal("handler не должен быть вызван")
			return nil, nil
		}

		wrappedHandler := ProjectPermissionsMiddleware(handler)
		request := createTestRequest(map[string]interface{}{})

		result, err := wrappedHandler(context.Background(), db, nil, user, request)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.IsError)
	})

	t.Run("пользователь не член проекта возвращает ошибку доступа", func(t *testing.T) {
		db := setupTestDB(t)

		userId := uuid.Must(uuid.NewV4())
		insertUser(t, db, userId)
		user := &dao.User{ID: userId}

		projectId := uuid.Must(uuid.NewV4())

		handler := func(ctx context.Context, db *gorm.DB, bl *business.Business, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			t.Fatal("handler не должен быть вызван")
			return nil, nil
		}

		wrappedHandler := ProjectPermissionsMiddleware(handler)
		request := createTestRequest(map[string]interface{}{
			"project_id": projectId.String(),
		})

		result, err := wrappedHandler(context.Background(), db, nil, user, request)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.IsError)
	})

	t.Run("пользователь член проекта - успех", func(t *testing.T) {
		db := setupTestDB(t)

		userId := uuid.Must(uuid.NewV4())
		insertUser(t, db, userId)
		user := &dao.User{ID: userId}

		projectId := uuid.Must(uuid.NewV4())
		memberId := uuid.Must(uuid.NewV4())
		insertProjectMember(t, db, memberId, userId, projectId, 10)

		handlerCalled := false
		handler := func(ctx context.Context, db *gorm.DB, bl *business.Business, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			handlerCalled = true
			pm := ctx.Value("projectMember")
			assert.NotNil(t, pm)
			return mcp.NewToolResultText("success"), nil
		}

		wrappedHandler := ProjectPermissionsMiddleware(handler)
		request := createTestRequest(map[string]interface{}{
			"project_id": projectId.String(),
		})

		result, err := wrappedHandler(context.Background(), db, nil, user, request)

		assert.NoError(t, err)
		assert.True(t, handlerCalled)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
	})
}

func TestGetStateList(t *testing.T) {
	t.Run("успешное получение статусов с группировкой", func(t *testing.T) {
		db := setupTestDB(t)

		userId := uuid.Must(uuid.NewV4())
		insertUser(t, db, userId)
		user := &dao.User{ID: userId}

		workspaceId := uuid.Must(uuid.NewV4())
		insertWorkspace(t, db, workspaceId, "test-ws")

		projectId := uuid.Must(uuid.NewV4())
		insertProject(t, db, projectId, workspaceId)

		// Создаем статусы в разных группах
		insertState(t, db, uuid.Must(uuid.NewV4()), "Backlog", "#ccc", "backlog", 1, projectId, workspaceId)
		insertState(t, db, uuid.Must(uuid.NewV4()), "In Progress", "#0f0", "started", 2, projectId, workspaceId)
		insertState(t, db, uuid.Must(uuid.NewV4()), "Done", "#00f", "completed", 3, projectId, workspaceId)

		request := createTestRequest(map[string]interface{}{
			"project_id": projectId.String(),
		})

		result, err := getStateList(context.Background(), db, nil, user, request)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)

		var statesMap map[string][]dto.StateLight
		content := result.Content[0].(mcp.TextContent)
		err = json.Unmarshal([]byte(content.Text), &statesMap)
		require.NoError(t, err)

		assert.Len(t, statesMap["backlog"], 1)
		assert.Len(t, statesMap["started"], 1)
		assert.Len(t, statesMap["completed"], 1)

		assert.Equal(t, "Backlog", statesMap["backlog"][0].Name)
		assert.Equal(t, "In Progress", statesMap["started"][0].Name)
		assert.Equal(t, "Done", statesMap["completed"][0].Name)
	})

	t.Run("фильтрация по search_query", func(t *testing.T) {
		db := setupTestDB(t)

		userId := uuid.Must(uuid.NewV4())
		insertUser(t, db, userId)
		user := &dao.User{ID: userId}

		workspaceId := uuid.Must(uuid.NewV4())
		insertWorkspace(t, db, workspaceId, "test-ws")

		projectId := uuid.Must(uuid.NewV4())
		insertProject(t, db, projectId, workspaceId)

		insertState(t, db, uuid.Must(uuid.NewV4()), "Backlog", "#ccc", "backlog", 1, projectId, workspaceId)
		insertState(t, db, uuid.Must(uuid.NewV4()), "In Progress", "#0f0", "started", 2, projectId, workspaceId)

		request := createTestRequest(map[string]interface{}{
			"project_id":   projectId.String(),
			"search_query": "progress",
		})

		result, err := getStateList(context.Background(), db, nil, user, request)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)

		var statesMap map[string][]dto.StateLight
		content := result.Content[0].(mcp.TextContent)
		err = json.Unmarshal([]byte(content.Text), &statesMap)
		require.NoError(t, err)

		_, hasBacklog := statesMap["backlog"]
		assert.False(t, hasBacklog)
		assert.Len(t, statesMap["started"], 1)
		assert.Equal(t, "In Progress", statesMap["started"][0].Name)
	})

	t.Run("пустой результат для проекта без статусов", func(t *testing.T) {
		db := setupTestDB(t)

		userId := uuid.Must(uuid.NewV4())
		insertUser(t, db, userId)
		user := &dao.User{ID: userId}

		workspaceId := uuid.Must(uuid.NewV4())
		insertWorkspace(t, db, workspaceId, "test-ws")

		projectId := uuid.Must(uuid.NewV4())
		insertProject(t, db, projectId, workspaceId)

		request := createTestRequest(map[string]interface{}{
			"project_id": projectId.String(),
		})

		result, err := getStateList(context.Background(), db, nil, user, request)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)

		var statesMap map[string][]dto.StateLight
		content := result.Content[0].(mcp.TextContent)
		err = json.Unmarshal([]byte(content.Text), &statesMap)
		require.NoError(t, err)

		assert.Empty(t, statesMap)
	})

	t.Run("регистронезависимый поиск", func(t *testing.T) {
		db := setupTestDB(t)

		userId := uuid.Must(uuid.NewV4())
		insertUser(t, db, userId)
		user := &dao.User{ID: userId}

		workspaceId := uuid.Must(uuid.NewV4())
		insertWorkspace(t, db, workspaceId, "test-ws")

		projectId := uuid.Must(uuid.NewV4())
		insertProject(t, db, projectId, workspaceId)

		insertState(t, db, uuid.Must(uuid.NewV4()), "In Progress", "#0f0", "started", 1, projectId, workspaceId)

		request := createTestRequest(map[string]interface{}{
			"project_id":   projectId.String(),
			"search_query": "PROGRESS",
		})

		result, err := getStateList(context.Background(), db, nil, user, request)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)

		var statesMap map[string][]dto.StateLight
		content := result.Content[0].(mcp.TextContent)
		err = json.Unmarshal([]byte(content.Text), &statesMap)
		require.NoError(t, err)

		assert.Len(t, statesMap["started"], 1)
	})

	t.Run("сортировка по sequence", func(t *testing.T) {
		db := setupTestDB(t)

		userId := uuid.Must(uuid.NewV4())
		insertUser(t, db, userId)
		user := &dao.User{ID: userId}

		workspaceId := uuid.Must(uuid.NewV4())
		insertWorkspace(t, db, workspaceId, "test-ws")

		projectId := uuid.Must(uuid.NewV4())
		insertProject(t, db, projectId, workspaceId)

		// Вставляем в обратном порядке
		insertState(t, db, uuid.Must(uuid.NewV4()), "Third", "#333", "started", 3, projectId, workspaceId)
		insertState(t, db, uuid.Must(uuid.NewV4()), "First", "#111", "started", 1, projectId, workspaceId)
		insertState(t, db, uuid.Must(uuid.NewV4()), "Second", "#222", "started", 2, projectId, workspaceId)

		request := createTestRequest(map[string]interface{}{
			"project_id": projectId.String(),
		})

		result, err := getStateList(context.Background(), db, nil, user, request)

		assert.NoError(t, err)
		assert.NotNil(t, result)

		var statesMap map[string][]dto.StateLight
		content := result.Content[0].(mcp.TextContent)
		err = json.Unmarshal([]byte(content.Text), &statesMap)
		require.NoError(t, err)

		assert.Len(t, statesMap["started"], 3)
		assert.Equal(t, "First", statesMap["started"][0].Name)
		assert.Equal(t, "Second", statesMap["started"][1].Name)
		assert.Equal(t, "Third", statesMap["started"][2].Name)
	})
}
