// DAO (Data Access Object) - предоставляет методы для взаимодействия с базой данных.
//
//	Содержит функции для получения данных об issue, их родителях и связанных issue, а также для проверки прав доступа пользователя к issue.
//
// Основные возможности:
//   - Получение корневого issue для заданного issue.
//   - Получение списка issue, являющихся потомками заданного issue.
//   - Получение списка issue, которые являются родителями заданного issue.
//   - Получение списка пользователей, имеющих права доступа к issue.
//   - Разделение строки запроса на несколько частей для обработки.
//   - Проверка прав доступа пользователя к issue.
package dao

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/config"
	"github.com/gofrs/uuid"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var db *gorm.DB

func TestMain(m *testing.M) {
	Config = config.ReadConfig()
	db, _ = gorm.Open(postgres.New(postgres.Config{
		DSN:                  Config.DatabaseDSN,
		PreferSimpleProtocol: false,
	}), &gorm.Config{
		TranslateError: true,
	})

	code := m.Run()
	os.Exit(code)
}

func TestReplaceType(t *testing.T) {
	assert.NoError(t, ReplaceColumnType(db, "issue_comments", "id", "uuid"))
}

func TestGetIssueRoot(t *testing.T) {
	// Setup
	var issues []Issue
	{
		var user User
		db.First(&user)

		var project Project
		db.First(&project)

		ids := []uuid.UUID{GenUUID(), GenUUID(), GenUUID()}
		issues = []Issue{
			{
				ID:              ids[0],
				WorkspaceId:     project.WorkspaceId,
				ProjectId:       project.ID,
				CreatedAt:       time.Now(),
				CreatedById:     user.ID,
				UpdatedById:     uuid.NullUUID{UUID: user.ID, Valid: true},
				Name:            "#1",
				DescriptionHtml: "",
			},
			{
				ID:              ids[1],
				WorkspaceId:     project.WorkspaceId,
				ProjectId:       project.ID,
				CreatedAt:       time.Now(),
				CreatedById:     user.ID,
				UpdatedById:     uuid.NullUUID{UUID: user.ID, Valid: true},
				ParentId:        uuid.NullUUID{UUID: ids[0], Valid: true},
				Name:            "#2",
				DescriptionHtml: "",
			},
			{
				ID:              ids[2],
				WorkspaceId:     project.WorkspaceId,
				ProjectId:       project.ID,
				CreatedAt:       time.Now(),
				CreatedById:     user.ID,
				UpdatedById:     uuid.NullUUID{UUID: user.ID, Valid: true},
				ParentId:        uuid.NullUUID{UUID: ids[1], Valid: true},
				Name:            "#3",
				DescriptionHtml: "",
			},
		}

		for _, issue := range issues {
			if err := CreateIssue(db, &issue); err != nil {
				t.Fatal(err)
			}
		}
		t.Cleanup(func() {
			db.Unscoped().
				Session(&gorm.Session{AllowGlobalUpdate: true}).
				Set("permanentDelete", true).
				Delete(issues)
		})
		t.Logf("Created issues: %s->%s->%s", issues[0].Name, issues[1].Name, issues[2].Name)
	}

	t.Log("Test #3 child issue")
	if root := GetIssueRoot(issues[2], db); root != nil {
		if root.ID != issues[0].ID {
			t.Errorf("Wrong root issue id: %s expect: %s", root.ID, issues[0].ID)
		}
		t.Logf("Root parent for %s is %s", issues[2].Name, root.Name)
	} else {
		t.Errorf("Issue has nil parent")
	}

	t.Log("Test #2 child issue")
	if root := GetIssueRoot(issues[1], db); root != nil {
		if root.ID != issues[0].ID {
			t.Errorf("Wrong root issue id: %s expect: %s", root.ID, issues[0].ID)
		}
		t.Logf("Root parent for %s is %s", issues[1].Name, root.Name)
	} else {
		t.Errorf("Issue has nil parent")
	}

	t.Log("Test root issue")
	if root := GetIssueRoot(issues[0], db); root != nil {
		t.Errorf("Root issue has parent: %s", root.ID)
	}
}

func TestGetIssueFamily(t *testing.T) {
	/* Setup
		Issues tree
		    #0      #6  ^
	 	   /  \         |
		  #1  #2        |
		 /    / \       |
		#3   #4  #5     |
	*/
	var issues []Issue
	{
		var user User
		db.First(&user)

		var project Project
		db.First(&project)

		for i := 0; i < 7; i++ {
			issues = append(issues, Issue{
				ID:              GenUUID(),
				WorkspaceId:     project.WorkspaceId,
				ProjectId:       project.ID,
				CreatedAt:       time.Now(),
				CreatedById:     user.ID,
				UpdatedById:     uuid.NullUUID{UUID: user.ID, Valid: true},
				Name:            fmt.Sprintf("#%d", i),
				DescriptionHtml: "",
			})
		}

		issues[1].ParentId = uuid.NullUUID{UUID: issues[0].ID, Valid: true}
		issues[2].ParentId = uuid.NullUUID{UUID: issues[0].ID, Valid: true}

		issues[3].ParentId = uuid.NullUUID{UUID: issues[1].ID, Valid: true}

		issues[4].ParentId = uuid.NullUUID{UUID: issues[2].ID, Valid: true}
		issues[5].ParentId = uuid.NullUUID{UUID: issues[2].ID, Valid: true}

		for _, issue := range issues {
			if err := CreateIssue(db, &issue); err != nil {
				t.Fatal(err)
			}
		}
		t.Cleanup(func() {
			db.Unscoped().
				Session(&gorm.Session{AllowGlobalUpdate: true}).
				Set("permanentDelete", true).
				Delete(issues)
		})
	}

	t.Logf("Middle issue test")
	if family := GetIssueFamily(issues[2], db); len(family) > 0 {
		familyStr := ""
		for _, member := range family {
			familyStr += member.Name + " "
		}
		t.Logf("Issue #2 family: %s", familyStr)
	} else {
		t.Error("Issue #2 has no family")
	}

	t.Logf("Root issue test")
	if family := GetIssueFamily(issues[0], db); len(family) > 0 {
		familyStr := ""
		for _, member := range family {
			familyStr += member.Name + " "
		}
		t.Logf("Issue #0 family: %s", familyStr)
	} else {
		t.Error("Issue #0 has no family")
	}

	t.Logf("Last issue test")
	if family := GetIssueFamily(issues[5], db); len(family) > 0 {
		familyStr := ""
		for _, member := range family {
			familyStr += member.Name + " "
		}
		t.Logf("Issue #5 family: %s", familyStr)
	} else {
		t.Error("Issue #5 has no family")
	}

	t.Logf("Orphan test")
	if family := GetIssueFamily(issues[6], db); len(family) > 1 {
		t.Errorf("Issue #6 has family: %v", family)
	} else {
		t.Logf("Issue #6 family: %s", family[0].Name)
	}
}

func TestSplitTSQuery(t *testing.T) {
	fmt.Println(SplitTSQuery("тензорный ускоритель"))
}

func TestGetUserPrivilegesOverIssue(t *testing.T) {
	userId := uuid.Must(uuid.FromString("cd61d7df-7025-4bf0-85f9-f374f5d10008"))
	priv, err := GetUserPrivilegesOverIssue("114c08ce-c9f5-4ca6-b829-63ec337a6238", userId, db)
	fmt.Println(err)
	fmt.Printf("%+v\n", priv)
}
