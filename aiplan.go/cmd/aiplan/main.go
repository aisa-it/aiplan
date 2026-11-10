// Основной пакет приложения AIPlan: разбор флагов и запуск ядра через пакет app. Также содержит логику для работы с Atlas.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/exec"

	"github.com/aisa-it/aiplan/aiplan.go/pkg/app"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/config"

	"ariga.io/atlas-go-sdk/atlasexec"
)

var version string = "DEV"

// main запускает ядро со штатным движком. Весь порядок запуска — в пакете app,
// чтобы сборки с другим движком вызывали его же, а не копировали.
//
// Пример запуска: go run main.go --noTranslate --noMigration --trace
func main() {
	opts := app.Options{Version: version}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	if err := app.Run(context.Background(), opts); err != nil {
		slog.Error("AIPlan fail", "err", err)
		os.Exit(1)
	}
}

// AtlasMigration Применяет изменения схемы базы данных через Atlas.  Использует указанные параметры конфигурации для подключения к базе данных и применения изменений схемы.  Обрабатывает ошибки и выводит информацию об успешно примененных изменениях.  Необходимо, чтобы утилита 'atlas' была доступна в системе.
//
// Параметры:
//   - cfg: Конфигурация приложения, содержащая параметры подключения к базе данных.
//
// Возвращает:
//   - error: Ошибка, если при возникновении проблем с подключением к Atlas или при применении изменений схемы.
func AtlasMigration(cfg *config.Config) error {
	_, err := exec.LookPath("atlas")
	if err != nil {
		slog.Warn("Atlas cli exec not found in system, skip schema applying", "err", err)
		return nil
	}

	client, err := atlasexec.NewClient(".", "atlas")
	if err != nil {
		return err
	}
	res, err := client.SchemaApply(context.Background(), &atlasexec.SchemaApplyParams{
		URL: cfg.DatabaseDSN,
		//DevURL:      cfg.DatabaseEmptyDSN,
		To:          "file://schema.sql",
		AutoApprove: true,
	})

	if res != nil && res.Changes.Error != nil {
		fmt.Println("Error statement:")
		fmt.Printf("%s:\n%s\n", res.Changes.Error.Text, res.Changes.Error.Stmt)
	}

	if err != nil {
		return err
	}

	if len(res.Changes.Applied) > 0 {
		fmt.Println("Applied changes:")
		for _, change := range res.Changes.Applied {
			fmt.Println(change)
		}
	}

	return nil
}
