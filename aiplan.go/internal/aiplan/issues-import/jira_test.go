// Пакет предоставляет функциональность для импорта и обработки информации об issue из Jira.
//
// Основные возможности:
//   - Получение URL Jira issue по идентификатору.
//   - Загрузка и обработка данных об issue из JSON файла.
//   - Сортировка issue по ParentId и SequenceId.
package issues_import

import (
	"fmt"
	"slices"
	"testing"

	"github.com/aisa-it/aiplan/internal/aiplan/config"
	"github.com/aisa-it/aiplan/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/internal/aiplan/issues-import/context"
	"github.com/aisa-it/aiplan/internal/aiplan/issues-import/entity"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestSingleIssue(t *testing.T) {
	jiraUrl := ""
	login := ""
	password := ""
	c, _ := newJiraClient(login, password, jiraUrl)
	cfg := config.ReadConfig()

	db, _ := gorm.Open(postgres.New(postgres.Config{
		DSN: cfg.DatabaseDSN,
	}))

	ic, err := context.NewImportContext(
		cfg.WebURL,
		c,
		db,
		nil,
		nil,
		dao.User{},
		"PORTAL",
		"c271525b-63dd-4659-a7a6-510d9338ba92",
		"10000",
		entity.NewLinkMapper("10003", "10400"),
		entity.PrioritiesMapping{},
	)
	if err != nil {
		t.Fatal(err)
	}

	ic.GetProject("PORTAL")

	_, err = ic.GetIssue("PORTAL-2456")
	fmt.Println(err)

	ic.ReleasesTags.Range(func(key string, value dao.Label) {
		fmt.Println(value.ID, value.Name)
	})
	ic.IssueLabels.Range(func(i int, il dao.IssueLabel) {
		fmt.Println(il)
	})
}

type test struct {
	Id       int
	ParentId int
}

func TestSort(t *testing.T) {
	issues := []test{
		{Id: 1},
		{Id: 2},
		{Id: 3, ParentId: 6},
		{Id: 4, ParentId: 1},
		{Id: 5, ParentId: 3},
		{Id: 6, ParentId: 7},
		{Id: 7, ParentId: 5},
	}

	slices.SortFunc(issues, func(a test, b test) int {
		if a.ParentId == 0 && b.ParentId != 0 {
			return -1
		} else if a.ParentId != 0 && b.ParentId == 0 {
			return 1
		}

		if a.ParentId == b.Id {
			return 1
		} else if b.ParentId == a.Id {
			return -1
		}
		return a.Id - b.Id
	})

	fmt.Println(issues)
}
