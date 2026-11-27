package server

import (
	"archive/zip"
	"compress/flate"
	"errors"
	"io"
	"net/http"
	"os"
	"time"

	apicontext "github.com/aisa-it/aiplan/aiplan.go/pkg/api-context"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/apierrors"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/aql"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/dao"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/dto"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/engine"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/search"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/utils"
	"github.com/labstack/echo/v4"
	"golang.org/x/sync/errgroup"
	"gorm.io/gorm"
)

// aqlErrorResponse — ошибка запроса AQL с местом для подсветки.
type aqlErrorResponse struct {
	apierrors.DefinedError
	AQL *aql.Error `json:"aql"`
}

// aqlError отвечает на ошибку разбора AQL: ошибку в запросе отдаёт с местом для подсветки,
// остальное - как EError (текст внутренней ошибки клиенту не уходит).
func aqlError(c echo.Context, err error) error {
	var aqlErr *aql.Error
	if errors.As(err, &aqlErr) && aqlErr.Code != aql.ErrInternal.Code {
		return c.JSON(http.StatusBadRequest, aqlErrorResponse{DefinedError: apierrors.ErrAQLQuery, AQL: aqlErr})
	}
	return EError(c, err)
}

// aqlIssuesQuery разбирает запрос AQL и навешивает видимость задач пользователя.
func (s *Services) aqlIssuesQuery(c echo.Context, query string) (*gorm.DB, error) {
	apiContext := apicontext.GetContext(c)
	ctx := c.Request().Context()
	scope := engine.IssueScope{Subject: apiContext, Kind: engine.ScopeGlobal}

	q, err := aql.ParseAQLQuery(
		s.DB(c),
		func(q *gorm.DB) *gorm.DB { return s.policy.ScopeIssues(ctx, scope, q) },
		s.policy.PropertyTemplateScope(ctx, apiContext),
		apiContext.GetUser(),
		query,
	)
	if err != nil {
		return nil, err
	}
	q = s.policy.ScopeIssues(ctx, scope, q)
	if apiContext.Error() != nil {
		return nil, apiContext.Error()
	}
	return q, nil
}

// searchIssues godoc
// @id searchIssues
// @Summary Задачи: поиск задач по AQL
// @Description Ищет задачи запросом на языке AQL среди всех видимых пользователю задач, например: priority = high ORDER BY priority DESC.
// @Description Сортировка задаётся в запросе через ORDER BY, по умолчанию - по номеру задачи по убыванию. Черновики в выдачу не попадают.
// @Tags Issues
// @Security ApiKeyAuth
// @Produce json
// @Param aql query string true "Запрос AQL, до 10000 байт"
// @Param offset query int false "Смещение для пагинации" default(0)
// @Param limit query int false "Лимит записей от 1 до 100, иначе 100" default(100)
// @Success 200 {object} dto.IssuesSearchResponse "Найденные задачи и их общее количество"
// @Failure 400 {object} aqlErrorResponse "Запрос не передан или содержит ошибку; место ошибки в запросе - в поле aql"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 408 {object} apierrors.DefinedError "Время ожидания запроса истекло"
// @Router /api/auth/search [get]
func (s *Services) searchIssues(c echo.Context) error {
	query := ""
	limit := 100
	offset := 0

	if err := echo.QueryParamsBinder(c).
		String("aql", &query).
		Int("limit", &limit).
		Int("offset", &offset).
		BindError(); err != nil {
		return EError(c, err)
	}

	if query == "" {
		return EErrorDefined(c, apierrors.ErrAQLQueryRequired)
	}

	if limit > 100 || limit < 1 {
		limit = 100
	}

	if offset < 0 {
		offset = 0
	}

	startMs := time.Now()

	q, err := s.aqlIssuesQuery(c, query)
	if err != nil {
		return aqlError(c, err)
	}

	var (
		issues []dao.IssueWithCount
		count  int64
		eg     errgroup.Group
	)

	q = q.Model(&dao.Issue{}).
		Preload("State").
		Preload("Project").
		Preload("Workspace").
		Preload("Assignees").
		Preload("Watchers").
		Preload("Labels").
		Preload("Parent").
		Session(&gorm.Session{})

	eg.Go(func() error {
		return q.Offset(offset).Limit(limit).Find(&issues).Error
	})

	eg.Go(func() error {
		return q.Select("issues.id").Count(&count).Error
	})

	if err := eg.Wait(); err != nil {
		return EError(c, err)
	}

	search.PopulateAuthors(issues)

	paginationMeta := dto.PaginationMeta{
		Count:  int(count),
		Offset: offset,
		Limit:  limit,
	}

	dtoIssues := utils.SliceToSlice(&issues, func(iwc *dao.IssueWithCount) dto.IssueWithCount { return *iwc.ToDTO() })
	return c.JSON(http.StatusOK, dto.IssuesSearchResponse{
		PaginationMeta: paginationMeta,
		Issues:         dtoIssues,
		Duration:       time.Since(startMs).Milliseconds(),
	})
}

// exportSearchIssues godoc
// @id exportSearchIssues
// @Summary Задачи: экспорт задач по AQL в CSV
// @Description Экспортирует найденные запросом AQL задачи в ZIP архив с файлом issues.csv. Сортировка - как в поиске, черновики не выгружаются.
// @Tags Issues
// @Security ApiKeyAuth
// @Produce application/zip
// @Param aql query string true "Запрос AQL, до 10000 байт"
// @Param include_properties query bool false "Добавить колонки дополнительных параметров задач" default(false)
// @Success 200 {file} binary "ZIP архив с CSV файлом"
// @Failure 400 {object} aqlErrorResponse "Запрос не передан или содержит ошибку; место ошибки в запросе - в поле aql"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 408 {object} apierrors.DefinedError "Время ожидания запроса истекло"
// @Router /api/auth/search/export [get]
func (s *Services) exportSearchIssues(c echo.Context) error {
	query := ""
	includeProperties := false

	if err := echo.QueryParamsBinder(c).
		String("aql", &query).
		Bool("include_properties", &includeProperties).
		BindError(); err != nil {
		return EError(c, err)
	}

	if query == "" {
		return EErrorDefined(c, apierrors.ErrAQLQueryRequired)
	}

	q, err := s.aqlIssuesQuery(c, query)
	if err != nil {
		return aqlError(c, err)
	}

	var issues []dao.IssueWithCount
	if err := q.Model(&dao.Issue{}).
		Preload("Workspace").
		Preload("State").
		Preload("Project").
		Preload("Assignees").
		Preload("Watchers").
		Preload("Labels").
		Preload("Sprints").
		Limit(issueExportLimit).
		Find(&issues).Error; err != nil {
		return EError(c, err)
	}

	search.PopulateAuthors(issues)

	res := dto.IssuesSearchResponse{
		Issues: utils.SliceToSlice(&issues, func(iwc *dao.IssueWithCount) dto.IssueWithCount { return *iwc.ToDTO() }),
	}

	var propsData *exportPropertiesData
	if includeProperties {
		propsData, err = s.loadExportProperties(c, res)
		if err != nil {
			return EError(c, err)
		}
	}

	f, err := os.CreateTemp("", "export-*.zip")
	if err != nil {
		return EError(c, err)
	}
	defer os.Remove(f.Name())

	z := zip.NewWriter(f)
	z.RegisterCompressor(zip.Deflate, func(w io.Writer) (io.WriteCloser, error) {
		return flate.NewWriter(w, flate.BestCompression)
	})

	entry, err := z.Create("issues.csv")
	if err != nil {
		return EError(c, err)
	}
	rows := make([]*dto.IssueWithCount, len(res.Issues))
	for i := range res.Issues {
		rows[i] = &res.Issues[i]
	}
	if err := writeIssuesCSV(entry, rows, propsData); err != nil {
		return EError(c, err)
	}

	if err := z.Close(); err != nil {
		return EError(c, err)
	}

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return EError(c, err)
	}

	c.Response().Header().Set("Content-Disposition", "attachment; filename=issues-export.zip")
	return c.Stream(http.StatusOK, "application/zip", f)
}
