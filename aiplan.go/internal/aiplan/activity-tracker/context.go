package tracker

import (
	"fmt"
	"maps"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/gofrs/uuid"
)

type ActivityCtx struct {
	Tracker    *ActTracker
	TrackerCtx *Ctx

	RequestedData   DataEntity
	CurrentInstance DataEntity
	Actor           *dao.User
	Layer           types.EntityLayer
}

func NewActCtx(t *ActTracker, ctx *Ctx, actor *dao.User, layer types.EntityLayer) *ActivityCtx {
	actCtx := ActivityCtx{Tracker: t, TrackerCtx: ctx, Actor: actor, Layer: layer}
	if ctx != nil {
		actCtx.RequestedData = ctx.MergeRequested()
		actCtx.CurrentInstance = ctx.MergeCurrent()
	}
	return &actCtx
}

func (c *ActivityCtx) Requested() Payload {
	return NewPayload(c.RequestedData)
}

func (c *ActivityCtx) Current() Payload {
	return NewPayload(c.CurrentInstance)
}

func (c *ActivityCtx) ResolveField(field actField.ActivityField) actField.ActivityField {
	field = c.applyScope(field)
	field = c.applyFieldLogOverride(field)
	return field
}

func (c *ActivityCtx) applyScope(field actField.ActivityField) actField.ActivityField {
	scope := c.scopeFromCurrent()
	if scope == "" {
		scope = c.scopeFromRequested()
	}

	if scope == "" {
		return field
	}

	return actField.ActivityField(fmt.Sprintf("%s_%s", scope, field))
}

func (c *ActivityCtx) scopeFromCurrent() string {
	scope, _ := GetAs[string](c.CurrentInstance, actField.UpdateScopeKey)
	return scope
}

func (c *ActivityCtx) scopeFromRequested() string {
	scope, _ := GetAs[string](c.RequestedData, actField.UpdateScopeKey)
	return scope
}

func (c *ActivityCtx) applyFieldLogOverride(field actField.ActivityField) actField.ActivityField {
	if override, ok := GetAs[actField.ActivityField](c.RequestedData, actField.NewKey(actField.KindLogOverride)); ok {
		return override
	}

	if override, ok := GetAs[actField.ActivityField](c.RequestedData, field.LogAs()); ok {
		return override
	}
	return field
}

func (c *ActivityCtx) getDiffData(act actField.FieldMapping) ([]interface{}, []interface{}) {
	f := GetAsOrDefault[string](c.RequestedData, act.Field.LookupFrom(), act.Field.String())
	newE := GetAsOrDefault[[]interface{}](c.RequestedData, actField.New(act.Req).AsKey(), []interface{}{})
	oldE := GetAsOrDefault[[]interface{}](c.CurrentInstance, actField.New(f).AsKey(), []interface{}{})
	return newE, oldE
}

type ActivitySide struct {
	data map[string]interface{}
}

func (s *ActivitySide) Set(field actField.ActivityField, kind actField.FieldKind, value any) {
	if value != nil {
		key := actField.FieldKey{Field: field, Kind: kind}
		s.data[key.String()] = value
	}
}
func (s *ActivitySide) SetKey(key actField.FieldKey, value any) {
	if value != nil {
		s.data[key.String()] = value
	}
}

func (s *ActivitySide) SetParentWithUUID(parentKey string, id *uuid.UUID) {
	s.Set(actField.ParentKey.Field, actField.KindEmpty, parentKey)
	if id != nil {
		s.SetKey(actField.NewKey(parentKey), *id)
	}
}

type Ctx struct {
	New ActivitySide
	Old ActivitySide

	GormMap    *map[string]interface{}
	OldGormMap *map[string]interface{}
}

func NewTrackerCtx(gormMap, oldGormMap *map[string]interface{}) *Ctx {
	return &Ctx{
		New:        ActivitySide{data: make(map[string]interface{})},
		Old:        ActivitySide{data: make(map[string]interface{})},
		GormMap:    gormMap,
		OldGormMap: oldGormMap,
	}
}

func (c *Ctx) mergeTwoMaps(primaryMap *map[string]interface{}, secondaryMap map[string]interface{}) map[string]interface{} {
	totalSize := len(secondaryMap)
	if primaryMap != nil {
		totalSize += len(*primaryMap)
	}

	result := make(map[string]interface{}, totalSize)

	if primaryMap != nil {
		maps.Copy(result, *primaryMap)
	}
	maps.Copy(result, secondaryMap)

	return result
}

func (c *Ctx) MergeRequested() map[string]interface{} {
	return c.mergeTwoMaps(c.GormMap, c.New.data)
}

func (c *Ctx) MergeCurrent() map[string]interface{} {
	return c.mergeTwoMaps(c.OldGormMap, c.Old.data)
}
