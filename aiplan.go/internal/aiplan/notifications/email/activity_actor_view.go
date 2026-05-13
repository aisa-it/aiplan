package email

import (
	"fmt"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/gofrs/uuid"
)

type ActivityActorView struct {
	Actors []dao.User

	AuthorsCount  int
	ActivityCount int
	CommentCount  int

	Start time.Time
	End   time.Time

	IsPeriod bool
}

func BuildActivityActorView(
	acts []FieldPrerender,
	tz *types.TimeZone,
) *ActivityActorView {

	authors := make(map[uuid.UUID]dao.User)

	var (
		start        time.Time
		end          time.Time
		init         bool
		count        int
		commentCount int
	)

	for _, fp := range acts {
		if fp.Count == 0 {
			continue
		}

		if fp.Field == actField.Comment.Field {
			commentCount += fp.Count
		} else {
			count += fp.Count
		}

		for _, author := range fp.Authors {
			if author.ID != uuid.Nil {
				authors[author.ID] = author
				fmt.Println("=", author.Avatar)
			}
		}

		if fp.Start.Valid {
			t := fp.Start.Time
			if !init {
				start, end = t, t
				init = true
			} else {
				if t.Before(start) {
					start = t
				}
				if t.After(end) {
					end = t
				}
			}
		}

		if fp.End.Valid {
			t := fp.End.Time
			if !init {
				start, end = t, t
				init = true
			} else {
				if t.Before(start) {
					start = t
				}
				if t.After(end) {
					end = t
				}
			}
		}
	}

	if count == 0 && commentCount == 0 {
		return nil
	}

	actors := make([]dao.User, 0, len(authors))
	for _, u := range authors {
		actors = append(actors, u)
	}

	loc := (*time.Location)(tz)
	start = start.In(loc)
	end = end.In(loc)

	// Определяем период (>= 2 секунд)
	isPeriod := false
	if !start.IsZero() && !end.IsZero() {
		isPeriod = end.Sub(start) >= 2*time.Second
	}

	return &ActivityActorView{
		Actors:        actors,
		AuthorsCount:  len(actors),
		ActivityCount: count,
		CommentCount:  commentCount,
		Start:         start,
		End:           end,
		IsPeriod:      isPeriod,
	}
}
