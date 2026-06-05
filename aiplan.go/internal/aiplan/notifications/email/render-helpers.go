package email

import (
	"database/sql"
	"strconv"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

type EntityChangeMeta struct {
	Authors map[uuid.UUID]dao.User
	Start   sql.NullTime
	End     sql.NullTime
	Verb    string
}

type entitySpec[E dao.IDaoAct] struct {
	entityID    func(dao.ActivityEvent) uuid.UUID
	entityTitle func(E) string
	loadRemoved func(*gorm.DB, []uuid.UUID) map[uuid.UUID]string
}

type DigestView struct {
	Title  string
	IsNew  bool
	IsGone bool
}

type DigestComplexView struct {
	ActivityMap map[uuid.UUID]dao.ActivityEvent
	Title       *string
	Old         *string
	New         *string
	TimeAction  *string
	IsNew       bool
	IsGone      bool
	IsUpdate    bool
	IsInfo      bool
	WithBlock   bool
}

func createFieldRenderer(label string, fieldType FieldType, opts ...RendererOption) FieldRenderer {
	config := &rendererConfig{}
	for _, opt := range opts {
		opt(config)
	}

	return func(_ *gorm.DB, t *EmailTemplates, acts []dao.ActivityEvent, _ dao.IDaoAct) FieldPrerender {
		var newV, oldV *actValueCtx
		n := acts[0].NewValue
		v := acts[0].OldValue
		if fieldType == EmojiField {
			if nInt, err := strconv.Atoi(n); err == nil {
				n = string(rune(nInt))
			}
			v = ""
		}

		if config.translationMap != nil {
			n = types.TranslateMap(config.translationMap, &n)
			v = types.TranslateMap(config.translationMap, &v)
		}

		if fieldType == BodyField {
			newV = toValueCtx(nil, &n)
			oldV = toValueCtx(nil, &v)
		} else {
			newV = toValueCtx(&n, nil)
			oldV = toValueCtx(&v, nil)
		}

		if config.customText != nil {
			newV = toValueCtx(config.customText, nil)
			oldV = nil
		}

		fp := t.RenderCollectOne(collectOneCtx{
			Key:    label,
			New:    newV,
			Old:    oldV,
			Start:  sql.NullTime{Time: acts[0].CreatedAt, Valid: true},
			Author: *acts[0].Actor,
		})
		fp.Verb = acts[0].Verb
		return fp
	}
}

// renderEntityChange
func renderEntityChange[E dao.IDaoAct](tx *gorm.DB, t *EmailTemplates, acts []dao.ActivityEvent, current []E, key string, spec entitySpec[E]) FieldPrerender {

	views, meta, count := buildEntityChange(tx, acts, current, spec)
	ctx := collectAllCtx{
		Key:    key,
		Views:  views,
		Start:  meta.Start,
		End:    meta.End,
		Author: meta.Authors,
	}
	p := t.RenderCollectAll(ctx, count)
	p.Add(optPrerenderAll)
	return p
}

func buildEntityChange[E dao.IDaoAct](tx *gorm.DB, activities []dao.ActivityEvent, current []E, spec entitySpec[E]) (
	views []DigestView, meta EntityChangeMeta, count int) {

	currentMap := make(map[uuid.UUID]E, len(current))

	for _, e := range current {
		currentMap[e.GetId()] = e
	}

	transitions, authors, start, end := collectTransitions(activities, spec.entityID)

	for _, entity := range current {
		t := transitions[entity.GetId()]

		view := DigestView{
			Title: spec.entityTitle(entity),
		}

		if t != nil && t.Created && !t.Deleted {
			view.IsNew = true
			count++
		}

		views = append(views, view)
	}

	var removedIDs []uuid.UUID
	for id, t := range transitions {
		if _, exists := currentMap[id]; exists {
			continue
		}

		if t.Deleted && !t.Created {
			removedIDs = append(removedIDs, id)
		}
	}

	if len(removedIDs) > 0 {
		titles := spec.loadRemoved(tx, removedIDs)

		for _, id := range removedIDs {
			title := titles[id]

			if title == "" {
				title = "unknown"
			}

			views = append(views, DigestView{
				Title:  title,
				IsGone: true,
			})

			count++
		}
	}

	meta.Authors = authors
	meta.Start = start
	meta.End = end

	return views, meta, count
}

func collectTransitions(activities []dao.ActivityEvent, entityID func(dao.ActivityEvent) uuid.UUID) (
	map[uuid.UUID]*transitionFlags, map[uuid.UUID]dao.User, sql.NullTime, sql.NullTime) {

	transitions := make(map[uuid.UUID]*transitionFlags)
	authors := make(map[uuid.UUID]dao.User)

	var (
		start   time.Time
		end     time.Time
		hasTime bool
	)

	for _, act := range activities {
		id := entityID(act)

		t := transitions[id]
		if t == nil {
			t = &transitionFlags{}
			transitions[id] = t
		}

		switch act.Verb {
		case actField.VerbAdded:
			if t.Deleted {
				t.Deleted = false
			} else {
				t.Created = true
			}

		case actField.VerbRemoved:
			if t.Created {
				t.Created = false
			} else {
				t.Deleted = true
			}
		case actField.VerbUpdated:
			if act.NewIdentifier.Valid {
				t.Created = true
			}
			if act.OldIdentifier.Valid {
				t.Deleted = true
			}
			if !act.NewIdentifier.Valid &&
				!act.OldIdentifier.Valid {
				t.Updated = true
			}
		}

		if act.Actor != nil {
			authors[act.Actor.ID] = *act.Actor
		}

		at := act.CreatedAt

		if !hasTime {
			start, end = at, at
			hasTime = true
		} else {
			if at.Before(start) {
				start = at
			}

			if at.After(end) {
				end = at
			}
		}
	}

	var ns, ne sql.NullTime

	if hasTime {
		ns = sql.NullTime{Time: start, Valid: true}
		ne = sql.NullTime{Time: end, Valid: true}
	}

	return transitions, authors, ns, ne
}

// -------

func makeEntityComplexRenderer(entityName string, opts ...RendererOption) func(tx *gorm.DB, t *EmailTemplates, acts []dao.ActivityEvent, entity dao.IDaoAct) FieldPrerender {
	return func(tx *gorm.DB, t *EmailTemplates, acts []dao.ActivityEvent, entity dao.IDaoAct) FieldPrerender {
		return renderEntityChangeComplex(tx, t, acts, entityName, opts...)
	}
}

func renderEntityChangeComplex(tx *gorm.DB, t *EmailTemplates, acts []dao.ActivityEvent, key string, opts ...RendererOption) FieldPrerender {
	config := rendererConfig{}
	for _, opt := range opts {
		opt(&config)
	}

	views, meta, count, replaceMap := buildComplexEntityChange(acts, opts...)
	m := make(map[uuid.UUID]string)
	for _, v := range views {
		p := t.RenderCollectValues(v)
		for u, _ := range v.ActivityMap {
			m[u] = p.Value
		}
	}
	ctx := collectComplexCtx{
		Key:    key,
		Value:  ApplyCustomReplaceText(complexActivities, replaceMap),
		Start:  meta.Start,
		End:    meta.End,
		Author: meta.Authors,
	}

	pp := t.RenderCollectComplex(ctx, count)
	if config.complexBlock {
		pp.Add(optComplexBlock)
	}
	pp.Replace = replaceMap
	pp.Verb = meta.Verb
	pp.ValueComplex = m
	pp.Add(optPrerenderComplex)
	return pp
}

// -------

func buildComplexEntityChange(activities []dao.ActivityEvent, opts ...RendererOption) (
	views []DigestComplexView, meta EntityChangeMeta, count int, replace map[string]any) {

	config := rendererConfig{}
	for _, opt := range opts {
		opt(&config)
	}

	agg := aggregateComplexChanges(activities, config)

	for _, change := range agg.Changes {
		switch {
		case change.Created && change.Deleted: // Created -> Deleted
			continue
		case change.Created: // Created -> Updated*
			views = append(views, DigestComplexView{
				Title:       change.Title,
				New:         change.LastNew,
				IsNew:       true,
				TimeAction:  change.TimeAction,
				WithBlock:   config.complexBlock,
				ActivityMap: change.ActivityMap,
			})
			count++
		case change.Deleted: // Updated* -> Deleted
			views = append(views, DigestComplexView{
				Title:       change.Title,
				Old:         change.FirstOld,
				IsGone:      true,
				TimeAction:  change.TimeAction,
				WithBlock:   config.complexBlock,
				ActivityMap: change.ActivityMap,
			})
			count++
		case change.Updated: // Updated*
			views = append(views, DigestComplexView{
				Title:       change.Title,
				Old:         change.FirstOld,
				New:         change.LastNew,
				IsUpdate:    true,
				TimeAction:  change.TimeAction,
				WithBlock:   config.complexBlock,
				ActivityMap: change.ActivityMap,
			})
			count++
		case change.Info:
			views = append(views, DigestComplexView{
				Title:       change.Title,
				New:         change.LastNew,
				IsInfo:      true,
				TimeAction:  change.TimeAction,
				WithBlock:   config.complexBlock,
				ActivityMap: change.ActivityMap,
			})
			count++
		}
	}

	meta.Authors = agg.Authors
	meta.Start = agg.Start
	meta.End = agg.End
	meta.Verb = activities[0].Verb

	if config.replaceMap == nil {
		config.replaceMap = make(map[string]any)
	}
	return views, meta, count, config.replaceMap
}

type digestAggregation struct {
	Changes map[uuid.UUID]*entityChange
	Authors map[uuid.UUID]dao.User

	Start sql.NullTime
	End   sql.NullTime
}

func aggregateComplexChanges(activities []dao.ActivityEvent, config rendererConfig) digestAggregation {
	result := digestAggregation{
		Changes: make(map[uuid.UUID]*entityChange),
		Authors: make(map[uuid.UUID]dao.User),
	}

	un := make(map[string]struct{})

	var (
		start   time.Time
		end     time.Time
		hasTime bool
	)

	for _, act := range activities {
		id, ok := activityEntityID(act)
		if !ok {
			switch act.Verb {
			case actField.VerbCreated:
				un[act.NewValue] = struct{}{}
				continue
			case actField.VerbDeleted:
				if _, exist := un[act.OldValue]; exist {
					continue
				}
			case actField.VerbUpdated:
				continue
			}
			id = dao.GenUUID()
		}

		newVal := &act.NewValue
		oldVal := &act.OldValue

		if config.replacer != nil {
			newVal = config.replacer(newVal)
			oldVal = config.replacer(oldVal)
		}

		change := result.Changes[id]
		if change == nil {
			m := make(map[uuid.UUID]dao.ActivityEvent)
			change = &entityChange{
				ID:          id,
				ActivityMap: m,
			}
			result.Changes[id] = change
		}

		if config.titleFunc != nil {
			change.Title = config.titleFunc(&act)
		}

		if config.timeActionFunc != nil {
			change.TimeAction = config.timeActionFunc(act)
		}

		if config.customComplexAggregateFunc != nil {
			config.customComplexAggregateFunc(change, act)
		} else {
			switch act.Verb {
			case actField.VerbCreated:
				change.Created = true
				if newVal != nil {
					v := *newVal
					change.LastNew = &v
				}
			case actField.VerbUpdated:
				change.Updated = true
				if change.FirstOld == nil && oldVal != nil {
					v := *oldVal
					change.FirstOld = &v
				}
				if newVal != nil {
					v := *newVal
					change.LastNew = &v
				}
			case actField.VerbDeleted:
				change.Deleted = true
				if change.FirstOld == nil && oldVal != nil {
					v := *oldVal
					change.FirstOld = &v
				}
			}
		}
		change.ActivityMap[act.ID] = act

		if act.Actor != nil {
			result.Authors[act.Actor.ID] = *act.Actor
		}

		at := act.CreatedAt

		if !hasTime {
			start = at
			end = at
			hasTime = true
		} else {
			if at.Before(start) {
				start = at
			}

			if at.After(end) {
				end = at
			}
		}
	}

	if hasTime {
		result.Start = sql.NullTime{Time: start, Valid: true}
		result.End = sql.NullTime{Time: end, Valid: true}
	}

	return result
}

func activityEntityID(act dao.ActivityEvent) (uuid.UUID, bool) {
	if act.NewIdentifier.Valid {
		return act.NewIdentifier.UUID, true
	}
	if act.OldIdentifier.Valid {
		return act.OldIdentifier.UUID, true
	}
	return uuid.Nil, false
}

func createTargetDateZRender(label string, template string) FieldRenderer {
	return func(_ *gorm.DB, t *EmailTemplates, acts []dao.ActivityEvent, _ dao.IDaoAct) FieldPrerender {
		replace := make(map[string]any)

		fp := t.RenderCollectOne(collectOneCtx{
			Key:    label,
			New:    ApplyCollectDate(&acts[0].NewValue, template+"_new", replace),
			Old:    ApplyCollectDate(&acts[0].OldValue, template+"_old", replace),
			Start:  sql.NullTime{Time: acts[0].CreatedAt, Valid: true},
			Author: *acts[0].Actor,
		})
		fp.Replace = replace
		fp.Verb = acts[0].Verb
		return fp
	}
}

func getUUIDFromActivity(newID, oldID *uuid.UUID) uuid.UUID {
	if newID != nil && *newID != uuid.Nil {
		return *newID
	}
	if oldID != nil && *oldID != uuid.Nil {
		return *oldID
	}
	return uuid.Nil
}

func getRemovedIssues(tx *gorm.DB, ids []uuid.UUID) map[uuid.UUID]string {
	return getRemovedEntities(tx.Joins("Project"), ids, func(a dao.Issue) string { return a.String() })
}

func getRemovedMembers(tx *gorm.DB, ids []uuid.UUID) map[uuid.UUID]string {
	return getRemovedEntities(tx, ids, func(a dao.User) string { return a.GetName() })
}

func getRemovedEntities[T dao.IDaoAct](tx *gorm.DB, ids []uuid.UUID, f func(a T) string) map[uuid.UUID]string {
	result := make(map[uuid.UUID]string)
	if len(ids) == 0 {
		return result
	}
	query := "id IN ?"

	var model T
	tableName := tx.Model(&model).Statement.Table
	if tableName == "" {
		stmt := &gorm.Statement{DB: tx}
		if err := stmt.Parse(&model); err == nil {
			tableName = stmt.Table
		}
	}

	if tableName != "" {
		query = tableName + "." + query
	}
	var entities []T

	tx.Unscoped().Where(query, ids).Find(&entities)

	for _, entity := range entities {
		result[entity.GetId()] = f(entity)
	}
	return result
}

func getAuthorTitle(act *dao.ActivityEvent) *string {
	if act.Actor != nil {
		return utils.ToPtr(act.Actor.GetName())
	}
	return nil
}
