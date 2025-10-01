// Содержит правила валидации и логики, применяемые при изменении статуса задачи.
//
// Основные возможности:
//   - Загрузка скриптов правил из базы данных.
//   - Выполнение скриптов правил перед изменением статуса задачи.
//   - Возврат результата выполнения скрипта (успех/неудача, сообщение об ошибке).
package rules

import (
	"fmt"
	"os"
	"testing"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/config"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var db *gorm.DB

func TestMain(m *testing.M) {
	dao.Config = config.ReadConfig()
	db, _ = gorm.Open(postgres.New(postgres.Config{
		DSN:                  dao.Config.DatabaseDSN,
		PreferSimpleProtocol: false,
	}), &gorm.Config{
		TranslateError: true,
	})

	code := m.Run()
	os.Exit(code)
}

func TestBeforeStatusChange(t *testing.T) {
	var issue dao.Issue
	db.Where("id = ?", "8e7c2226-5caf-40b2-b7b6-d0f68e501c20").Preload(clause.Associations).Find(&issue)

	script := `
	function BeforeStatusChange(params, newstatus)
		if newstatus.name == "Выполнена" then
        	return { status = false, error = "У вас нет прав переводить в этот статус." }
    	end

    	return { status = true }
	end
	`

	issue.Project.RulesScript = &script

	fmt.Println(BeforeStatusChange(*issue.Author, issue, *issue.State))
}
