// Обработка редиректов для коротких ссылок на задачи и документы.
//
// Основные возможности:
//   - Редирект коротких ссылок на задачи по проектам.
//   - Редирект коротких ссылок на документы.
package aiplan

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/labstack/echo/v4"
)

func (s *Services) shortIssueURLRedirect(c echo.Context) error {
	var slug, projectIdent, issueNum string
	if c.Path() == "/i/:slug/:projectIdent/:issueNum/" {
		slug = c.Param("slug")
		projectIdent = c.Param("projectIdent")
		issueNum = c.Param("issueNum")
	} else {
		arr := strings.Split(c.Param("issue"), "-")
		if len(arr) != 3 {
			return c.Redirect(http.StatusTemporaryRedirect, "/not-found/")
		}
		slug = arr[0]
		projectIdent = arr[1]
		issueNum = arr[2]
	}

	var issue dao.Issue
	if err := s.db.
		Joins("Workspace").
		Joins("Project").
		Where("slug = ?", slug).
		Where("identifier = ?", projectIdent).
		Where("sequence_id = ?", issueNum).
		First(&issue).Error; err != nil {
		return c.Redirect(http.StatusTemporaryRedirect, "/not-found/")
	}

	ref, _ := url.Parse(fmt.Sprintf(
		"/%s/projects/%s/issues/%d/",
		slug,
		issue.Project.Identifier,
		issue.SequenceId))
	path := cfg.WebURL.ResolveReference(ref)
	return c.Redirect(http.StatusTemporaryRedirect, path.String())
}

func (s *Services) shortDocURLRedirect(c echo.Context) error {
	slug := c.Param("slug")
	docNum := c.Param("docNum")

	var doc dao.Doc
	if err := s.db.
		Joins("Workspace").
		Where("slug = ?", slug).
		Where("docs.id = ?", docNum).
		First(&doc).Error; err != nil {
		return c.Redirect(http.StatusTemporaryRedirect, "/not-found/")
	}

	ref, _ := url.Parse(fmt.Sprintf(
		"/%s/aidoc/%s/",
		slug,
		doc.ID.String()))
	path := cfg.WebURL.ResolveReference(ref)
	return c.Redirect(http.StatusTemporaryRedirect, path.String())
}

func (s *Services) shortSearchFilterURLRedirect(c echo.Context) error {
	base := c.Param("base")

	id, err := utils.Base64ToUUID(base)
	if err != nil {
		return c.Redirect(http.StatusTemporaryRedirect, "/not-found/")
	}

	var sf dao.SearchFilter
	if err := s.db.
		Where("id = ?", id).Where("public = ?", true).
		First(&sf).Error; err != nil {
		return c.Redirect(http.StatusTemporaryRedirect, "/not-found/")
	}

	ref, _ := url.Parse(fmt.Sprintf(
		"/filters/%s/",
		id.String(),
	))
	path := cfg.WebURL.ResolveReference(ref)
	return c.Redirect(http.StatusTemporaryRedirect, path.String())
}
