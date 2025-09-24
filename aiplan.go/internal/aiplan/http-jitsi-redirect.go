package aiplan

import (
	"net/http"
	"net/url"

	"github.com/labstack/echo/v4"
)

func (s *Services) redirectToJitsiConf(c echo.Context) error {
	user := c.(AuthContext).User
	room := c.Param("room")

	token, err := s.jitsiTokenIss.IssueToken(user, false, room)
	if err != nil {
		return EError(c, err)
	}

	q := make(url.Values)
	q.Add("jwt", token)
	u := cfg.JitsiURL.ResolveReference(&url.URL{Path: room, RawQuery: q.Encode()})

	return c.Redirect(http.StatusTemporaryRedirect, u.String())
}
