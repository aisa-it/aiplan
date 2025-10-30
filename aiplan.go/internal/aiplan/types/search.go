package types

import (
	"github.com/labstack/echo/v4"
)

type SearchParams struct {
	ShowSubIssues bool
	Draft         bool
	OrderByParam  string
	GroupByParam  string
	OnlyCount     bool
	Offset        int
	Limit         int
	Desc          bool
	LightSearch   bool
	OnlyActive    bool
	OnlyPinned    bool

	Filters IssuesListFilters
}

func ParseSearchParams(c echo.Context) (*SearchParams, error) {
	sp := &SearchParams{}
	if err := echo.QueryParamsBinder(c).
		Bool("show_sub_issues", &sp.ShowSubIssues).
		Bool("draft", &sp.Draft).
		String("order_by", &sp.OrderByParam).
		String("group_by", &sp.GroupByParam).
		Int("offset", &sp.Offset).
		Int("limit", &sp.Limit).
		Bool("desc", &sp.Desc).
		Bool("only_count", &sp.OnlyCount).
		Bool("light", &sp.LightSearch).
		Bool("only_active", &sp.OnlyActive).
		Bool("only_pinned", &sp.OnlyPinned).
		BindError(); err != nil {
		return nil, err
	}

	if err := c.Bind(&sp.Filters); err != nil {
		return nil, err
	}
	return sp, nil
}
