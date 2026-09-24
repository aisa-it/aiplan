package dao

import (
	"net/url"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/pkg/dto"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/types"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/utils"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

type PersonalPage struct {
	ID uuid.UUID `gorm:"column:id;primaryKey;type:uuid"`

	CreatedAt time.Time
	OwnerId   uuid.UUID `gorm:"type:uuid;index"`
	Owner     *User     `gorm:"foreignKey:OwnerId;references:ID;belongsTo" extensions:"x-nullable"`

	UpdatedAt   time.Time
	UpdatedById uuid.NullUUID `gorm:"type:uuid" extensions:"x-nullable"`
	UpdatedBy   *User         `gorm:"foreignKey:UpdatedById;references:ID;belongsTo" extensions:"x-nullable"`

	Tokens types.TsVector `gorm:"index:personal_page_tokens_gin,type:gin;->:false"`

	Title   string `validate:"required,max=150"`
	Content types.RedactorHTML

	InlineAttachments []FileAsset `gorm:"foreignKey:DocId"`

	Editors []User `gorm:"-"`
	Readers []User `gorm:"-"`

	EditorsIDs []uuid.UUID `gorm:"-"`
	ReaderIDs  []uuid.UUID `gorm:"-"`

	AccessRules []PersonalPageAccessRules `gorm:"foreignKey:DocId"`

	URL *url.URL `gorm:"-"`
}

func (d *PersonalPage) ToDTO() *dto.PersonalPage {
	if d == nil {
		return nil
	}

	return &dto.PersonalPage{
		ID:                d.ID,
		CreatedAt:         d.CreatedAt,
		OwnerId:           d.OwnerId,
		Owner:             d.Owner.ToLightDTO(),
		UpdatedAt:         d.UpdatedAt,
		UpdatedById:       d.UpdatedById,
		UpdatedBy:         d.UpdatedBy.ToLightDTO(),
		Title:             d.Title,
		Content:           d.Content,
		InlineAttachments: utils.SliceToSlice(&d.InlineAttachments, func(f *FileAsset) dto.FileAsset { return *f.ToDTO() }),
		Editors:           utils.SliceToSlice(&d.Editors, func(u *User) dto.UserLight { return *u.ToLightDTO() }),
		Readers:           utils.SliceToSlice(&d.Readers, func(u *User) dto.UserLight { return *u.ToLightDTO() }),
		EditorsIDs:        d.EditorsIDs,
		ReaderIDs:         d.ReaderIDs,
	}
}

func (d *PersonalPage) BeforeDelete(tx *gorm.DB) error {
	return tx.Where("page_id = ?", d.ID).Delete(&PersonalPageAccessRules{}).Error
}

func (d *PersonalPage) PopulateAccessFields() {
	d.Editors = make([]User, 0)
	d.Readers = make([]User, 0)
	d.EditorsIDs = make([]uuid.UUID, 0)
	d.ReaderIDs = make([]uuid.UUID, 0)

	if len(d.AccessRules) == 0 {
		return
	}

	for _, rule := range d.AccessRules {
		if rule.Edit {
			d.EditorsIDs = append(d.EditorsIDs, rule.UserId)
			if rule.User != nil {
				d.Editors = append(d.Editors, *rule.User)
			}
			// Редакторы по дефолту могут читать поэтому не добавляем их в список читателей
			continue
		}

		d.ReaderIDs = append(d.ReaderIDs, rule.UserId)
		if rule.User != nil {
			d.Readers = append(d.Readers, *rule.User)
		}
	}
}

type PersonalPageAccessRules struct {
	Id        uuid.UUID `gorm:"primaryKey"`
	CreatedAt time.Time
	UpdatedAt time.Time

	PageId uuid.UUID `gorm:"index;uniqueIndex:page_member_idx,priority:1"`
	UserId uuid.UUID `gorm:"uniqueIndex:page_member_idx,priority:2"`

	Edit bool

	Page *PersonalPage `gorm:"foreignKey:PageId" extensions:"x-nullable"`
	User *User         `gorm:"foreignKey:UserId;belongsTo" extensions:"x-nullable"`
}

// Возвращает имя таблицы, соответствующей сущности Doc. Используется для определения имени таблицы при работе с базой данных.
func (PersonalPageAccessRules) TableName() string { return "personal_page_access_rules" }
