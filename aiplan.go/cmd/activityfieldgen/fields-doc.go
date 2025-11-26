package main

import (
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
)

var (
	DocMemberExtendFields = StructInfo{
		Name: "DocMemberExtendFields",
		Fields: []StructField{
			{Name: "NewDocWatcher", Type: getTypePtr(dao.User{}), FieldTag: withTable(activities.FieldWatchers, dao.Doc{})},
			{Name: "OldDocWatcher", Type: getTypePtr(dao.User{}), FieldTag: withTable(activities.FieldWatchers, dao.Doc{})},

			{Name: "NewDocReader", Type: getTypePtr(dao.User{}), FieldTag: activities.FieldReaders},
			{Name: "OldDocReader", Type: getTypePtr(dao.User{}), FieldTag: activities.FieldReaders},

			{Name: "NewDocEditor", Type: getTypePtr(dao.User{}), FieldTag: activities.FieldEditors},
			{Name: "OldDocEditor", Type: getTypePtr(dao.User{}), FieldTag: activities.FieldEditors},
		},
	}

	DocAttachmentExtendFields = StructInfo{
		Name: "DocAttachmentExtendFields",
		Fields: []StructField{
			{Name: "NewDocAttachment", Type: getTypePtr(dao.DocAttachment{}), FieldTag: withTable(activities.FieldAttachment, dao.Doc{})},
			{Name: "OldDocAttachment", Type: getTypePtr(dao.DocAttachment{}), FieldTag: withTable(activities.FieldAttachment, dao.Doc{})},
		},
	}

	DocCommentExtendFields = StructInfo{
		Name: "DocCommentExtendFields",
		Fields: []StructField{
			{Name: "NewDocComment", Type: getTypePtr(dao.DocComment{}), FieldTag: withTable(activities.FieldComment, dao.Doc{})},
		},
	}

	DocExtendFields = StructInfo{
		Name: "DocExtendFields",
		Fields: []StructField{
			{Name: "NewDoc", Type: getTypePtr(dao.Doc{}), FieldTag: activities.FieldDoc},
			{Name: "OldDoc", Type: getTypePtr(dao.Doc{}), FieldTag: activities.FieldDoc},
		},
	}
)
