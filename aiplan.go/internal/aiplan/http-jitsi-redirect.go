package aiplan

import (
	"net/http"
	"net/url"

	apicontext "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/api-context"
	"github.com/labstack/echo/v4"
)

func (s *Services) redirectToJitsiConf(c echo.Context) error {
	if cfg.JitsiDisabled {
		return c.NoContent(http.StatusNotFound)
	}

	user := apicontext.GetContext(c).GetUser()
	room := c.Param("room")

	token, err := s.jitsiTokenIss.IssueToken(user, false, room)
	if err != nil {
		return EError(c, err)
	}

	q := make(url.Values)
	q.Add("jwt", token)
	u := cfg.JitsiURL.URL.ResolveReference(&url.URL{Path: room, RawQuery: q.Encode()})

	return c.Redirect(http.StatusTemporaryRedirect, u.String())
}
