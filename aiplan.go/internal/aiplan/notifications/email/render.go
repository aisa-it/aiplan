package email

import (
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/gofrs/uuid"
)

type transitionFlags struct {
	Created bool
	Deleted bool
	Updated bool

	Added   bool
	Removed bool

	Info bool
}

type entityChange struct {
	transitionFlags

	ID          uuid.UUID
	ActivityMap map[uuid.UUID]dao.ActivityEvent

	FirstOld *string
	LastNew  *string

	Title      *string
	TimeAction *string
}

type funcReplacer func(*string) *string

type rendererConfig struct {
	translationMap             map[string]string
	titleFunc                  func(act *dao.ActivityEvent) *string
	timeActionFunc             func(act dao.ActivityEvent) *string
	replacer                   funcReplacer
	replaceMap                 map[string]any
	customValFunc              func(act dao.ActivityEvent) (string, string)
	complexBlock               bool
	customText                 *string
	customComplexAggregateFunc func(c *entityChange, act dao.ActivityEvent)
	formatInfoFunc             func(act dao.ActivityEvent) string
}

type RendererOption func(*rendererConfig)

type FieldType int

const (
	StringField FieldType = iota
	BodyField
	EmojiField
	TranslateField
)

func WithCustomText(str string) RendererOption {
	return func(c *rendererConfig) {
		c.customText = utils.ToPtr(str)
	}
}

func WithComplexAggregateFunc(f func(c *entityChange, act dao.ActivityEvent)) RendererOption {
	return func(c *rendererConfig) {
		c.customComplexAggregateFunc = f
		c.complexBlock = true
	}
}

func WithComplexBlock() RendererOption {
	return func(c *rendererConfig) {
		c.complexBlock = true
	}
}

func WithTranslation(translationMap map[string]string) RendererOption {
	return func(c *rendererConfig) {
		c.translationMap = translationMap
	}
}

func WithActionTime(template string) RendererOption {
	return func(c *rendererConfig) {
		if c.replaceMap == nil {
			c.replaceMap = make(map[string]any)
		}
		c.timeActionFunc = func(act dao.ActivityEvent) *string {
			return ApplyCollectDate(utils.ToPtr(act.CreatedAt.Format(time.RFC3339)), template+"_"+dao.GenUUID().String(), c.replaceMap).Value
		}
	}
}

func WithTitleFunc(f func(act *dao.ActivityEvent) *string) RendererOption {
	return func(c *rendererConfig) {
		c.titleFunc = f
	}
}

func WithReplaceHtml() RendererOption {
	return func(config *rendererConfig) {
		config.replacer = htmlReplacer
	}
}

func WithReplaceFunc(f func(str *string) *string) RendererOption {
	return func(config *rendererConfig) {
		config.replacer = f
	}
}
