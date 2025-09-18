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

	u, err := url.Parse(cfg.JitsiURL)
	if err != nil {
		return EError(c, err)
	}
	u.Path = room
	q := u.Query()
	q.Add("jwt", token)
	u.RawQuery = q.Encode()

	return c.Redirect(http.StatusTemporaryRedirect, u.String())
}
