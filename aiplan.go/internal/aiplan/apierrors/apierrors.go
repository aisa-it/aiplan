// Пакет содержит определения ошибок, используемых в приложении aiplan для обработки различных ситуаций, возникающих при работе с базой данных, внешними сервисами и пользовательским интерфейсом.  Каждая ошибка имеет код, статус HTTP и описание, что позволяет удобно обрабатывать исключения и предоставлять информативные сообщения пользователю.  Также включает в себя helper-функцию для форматирования сообщений об ошибках.
//
// Основные возможности:
//   - Определение различных типов ошибок, связанных с авторизацией, сессиями, рабочими пространствами, проектами, формами, пользователями, интеграциями, импортом, административными функциями и другими аспектами приложения.
//   - Предоставление кодов ошибок, соответствующих кодам HTTP статусов.
//   - Включение сообщений об ошибках для удобной обработки и отображения пользователю.
//   - Функция для форматирования сообщений об ошибках с использованием аргументов.
package apierrors

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	tusd "github.com/tus/tusd/v2/pkg/handler"
)

type DefinedError struct {
	Code       int    `json:"code"`
	StatusCode int    `json:"-"`
	Err        string `json:"error"`
	RuErr      string `json:"ru_error,omitempty"`
}

func (e DefinedError) Error() string {
	return e.Err
}

func (e DefinedError) TusdError() tusd.Error {
	b, _ := json.Marshal(e)
	return tusd.Error{
		HTTPResponse: tusd.HTTPResponse{
			StatusCode: e.StatusCode,
			Body:       string(b),
			Header: tusd.HTTPHeader{
				"Content-Type": "application/json",
			},
		},
	}
}

const (
	AttachmentsZipMaxSizeMB = 500
)

var (
	// 1*** - auth errors
	ErrFailedLogin              = DefinedError{Code: 1001, StatusCode: http.StatusUnauthorized, Err: "invalid credentials", RuErr: "Неправильный email или пароль"}
	ErrCaptchaFail              = DefinedError{Code: 1002, StatusCode: http.StatusUnauthorized, Err: "invalid captcha", RuErr: "Капча введена неверно"}
	ErrLoginCredentialsRequired = DefinedError{Code: 1003, StatusCode: http.StatusUnauthorized, Err: "both email and password are required", RuErr: "Поля email и пароль не могут быть пустыми"}
	ErrLoginTriesExceed         = DefinedError{Code: 1004, StatusCode: http.StatusUnauthorized, Err: "login tries exceed, your account is blocked", RuErr: "Учетная запись заблокирована"}
	ErrIntegrationLogin         = DefinedError{Code: 1005, StatusCode: http.StatusUnauthorized, Err: "integration login is prohibited", RuErr: "Некорректные данные авторизации"}
	ErrSignupDisabled           = DefinedError{Code: 1006, StatusCode: http.StatusForbidden, Err: "sign up disabled", RuErr: "Регистрация отключена администратором"}
	ErrAccessTokenRequired      = DefinedError{Code: 1007, Err: "access token is required", RuErr: "Требуется токен доступа"}
	ErrUserAlreadyExist         = DefinedError{Code: 1008, Err: "user already exist", RuErr: "Пользователь с указанным email уже зарегистрирован в системе"}
	ErrBlockedUntil             = DefinedError{Code: 1009, StatusCode: http.StatusUnauthorized, Err: "blocked until %s", RuErr: "Учетная запись заблокирована до %s"}
	ErrNewUserMailFailed        = DefinedError{Code: 1010, Err: "failed to deliver email with password to new user", RuErr: "Не удалось отправить пароль на указанную почту. Проверьте корректность указанного адреса"}

	// 11** - session errors
	ErrRefreshTokenRequired = DefinedError{Code: 1101, StatusCode: http.StatusUnauthorized, Err: "refresh token is required", RuErr: "Требуется токен обновления"}
	ErrTokenExpired         = DefinedError{Code: 1102, StatusCode: http.StatusUnauthorized, Err: "token expired", RuErr: "Срок действия токена истек"}
	ErrTokenInvalid         = DefinedError{Code: 1103, StatusCode: http.StatusUnauthorized, Err: "invalid token", RuErr: "Неверный токен"}
	ErrSessionReset         = DefinedError{Code: 1104, StatusCode: http.StatusUnauthorized, Err: "user session reset", RuErr: "Пользовательская сессия сброшена"}

	// 2*** - workspace errors
	ErrWorkspaceConflict                 = DefinedError{Code: 2001, StatusCode: http.StatusConflict, Err: "workspace already exists", RuErr: "Такое рабочее пространство уже существует"}
	ErrWorkspaceAdminNotFound            = DefinedError{Code: 2002, StatusCode: http.StatusNotFound, Err: "workspace admin not found", RuErr: "Администратор рабочей области не найден"}
	ErrPermissionChangeWorkspaceOwner    = DefinedError{Code: 2003, StatusCode: http.StatusForbidden, Err: "change of workspace owner is forbidden", RuErr: "У вас недостаточно прав для изменения владельца рабочего пространства"}
	ErrDeleteWorkspaceForbidden          = DefinedError{Code: 2004, StatusCode: http.StatusForbidden, Err: "deletion of workspace is forbidden", RuErr: "У вас недостаточно прав на удаление рабочего пространства"}
	ErrWorkspaceNotFound                 = DefinedError{Code: 2005, StatusCode: http.StatusNotFound, Err: "workspace not found", RuErr: "Рабочее пространство не найдено"}
	ErrUpdateOwnerForbidden              = DefinedError{Code: 2006, StatusCode: http.StatusForbidden, Err: "update of owner user is forbidden", RuErr: "У вас недостаточно прав на изменение владельца пространства"}
	ErrUpdateOwnUserForbidden            = DefinedError{Code: 2007, StatusCode: http.StatusForbidden, Err: "updating your own user is forbidden", RuErr: "У вас недостаточно прав на изменение собственных параметров"}
	ErrUpdateHigherRoleUserForbidden     = DefinedError{Code: 2008, StatusCode: http.StatusForbidden, Err: "cannot update user with a higher role than your own", RuErr: "У вас недостаточно прав для изменения пользователя с более высокой ролью, чем ваша"}
	ErrNotEnoughRights                   = DefinedError{Code: 2009, StatusCode: http.StatusForbidden, Err: "not enough rights", RuErr: "У вас недостаточно прав для выполнения этого действия"}
	ErrMemberAlreadyHasEmail             = DefinedError{Code: 2010, StatusCode: http.StatusBadRequest, Err: "member already has email", RuErr: "Поле Email не может быть пустым"}
	ErrCannotRemoveHigherRoleUser        = DefinedError{Code: 2011, StatusCode: http.StatusForbidden, Err: "you cannot remove a user having a higher role than you", RuErr: "У вас недостаточно прав для удаления пользователя с более высокой ролью, чем ваша"}
	ErrWorkspaceNameRequired             = DefinedError{Code: 2012, StatusCode: http.StatusBadRequest, Err: "workspace must have a name", RuErr: "Поле Имя проекта не может быть пустым"}
	ErrForbiddenSlug                     = DefinedError{Code: 2013, StatusCode: http.StatusBadRequest, Err: "forbidden slug", RuErr: "Индикатор содержит недопустимые символы"}
	ErrWorkspaceSlugConflict             = DefinedError{Code: 2014, StatusCode: http.StatusConflict, Err: "workspace with that slug already exists", RuErr: "Пространство с таким идентификатором уже существует"}
	ErrCannotUpdateWorkspaceAdmin        = DefinedError{Code: 2015, StatusCode: http.StatusForbidden, Err: "you cannot update workspace admin", RuErr: "У вас недостаточно прав на изменение администратора пространства"}
	ErrCannotDeleteWorkspaceAdmin        = DefinedError{Code: 2016, StatusCode: http.StatusForbidden, Err: "you cannot delete workspace admin", RuErr: "У вас недостаточно прав на удаление администратора пространства"}
	ErrTargetWorkspaceNotFoundOrNotAdmin = DefinedError{Code: 2017, StatusCode: http.StatusNotFound, Err: "target workspace does not exist or you are not an admin", RuErr: "Импортированное рабочее пространство не существует или вы не являетесь администратор"}
	ErrWorkspaceRoleRequired             = DefinedError{Code: 2018, StatusCode: http.StatusBadRequest, Err: "workspace role must be specified", RuErr: "Указана некорректная роль участника"}
	ErrInviteMemberExist                 = DefinedError{Code: 2019, StatusCode: http.StatusBadRequest, Err: "workspace member already is exists", RuErr: "Пользователь уже является участником данного пространства"}
	ErrCannotRemoveSelfFromWorkspace     = DefinedError{Code: 2020, StatusCode: http.StatusBadRequest, Err: "you cannot remove yourself from the workspace", RuErr: "У вас недостаточно прав на удаление себя из пространства"}
	ErrWorkspaceLimitExceed              = DefinedError{Code: 2021, StatusCode: http.StatusPaymentRequired, Err: "workspace limit exceed", RuErr: "Количество ваших пространств достигло лимита бесплатной версии"}
	ErrWorkspaceAdminRoleRequired        = DefinedError{Code: 2030, StatusCode: http.StatusForbidden, Err: "workspace admin role is required", RuErr: "У вас недостаточно прав. Для действия необходима роль администратора пространства"}
	ErrWorkspaceMemberNotFound           = DefinedError{Code: 2031, StatusCode: http.StatusBadRequest, Err: "workspace member not found", RuErr: "Пользователь не является участником данного пространства"}
	ErrWorkspaceForbidden                = DefinedError{Code: 2032, StatusCode: http.StatusForbidden, Err: "not have permissions to perform this action", RuErr: "Недостаточно прав для совершения действия"}
	ErrIntegrationNotFound               = DefinedError{Code: 2033, StatusCode: http.StatusNotFound, Err: "integration not found", RuErr: "Запрашиваемой интеграции не существует"}
	ErrChangeWorkspaceOwner              = DefinedError{Code: 2034, StatusCode: http.StatusBadRequest, Err: "error change of workspace owner", RuErr: "Не получилось изменить владельца пространства"}
	ErrDeleteLastWorkspaceMember         = DefinedError{Code: 2035, StatusCode: http.StatusBadRequest, Err: "cannot delete the last workspace member", RuErr: "Невозможно удалить последнего участника пространства"}
	ErrInvitesExceed                     = DefinedError{Code: 2036, StatusCode: http.StatusPaymentRequired, Err: "you invites limit exceed", RuErr: "Лимит на участников пространства исчерпан. Обновите ваш тарифный план для подключения дополнительный участников"}

	// 3*** - project errors
	ErrProjectConflict                   = DefinedError{Code: 3001, StatusCode: http.StatusConflict, Err: "project already exists", RuErr: "Такой проект уже существует"}
	ErrProjectAdminNotFound              = DefinedError{Code: 3002, StatusCode: http.StatusNotFound, Err: "project admin not found", RuErr: "Администратор проекта не найден"}
	ErrChangeProjectLeadForbidden        = DefinedError{Code: 3003, StatusCode: http.StatusForbidden, Err: "change of project lead is forbidden", RuErr: "У вас недостаточно прав на изменение лидера проекта"}
	ErrDeleteProjectForbidden            = DefinedError{Code: 3004, StatusCode: http.StatusForbidden, Err: "deletion of project is forbidden", RuErr: "У вас недостаточно прав на удаление проекта"}
	ErrProjectIdentifierRequired         = DefinedError{Code: 3005, StatusCode: http.StatusBadRequest, Err: "project identifier is required", RuErr: "Поле индикатор проекта не может быть пустым"}
	ErrProjectIdentifierConflict         = DefinedError{Code: 3006, StatusCode: http.StatusConflict, Err: "project with this identifier exists", RuErr: "Проект с таким идентификатором уже существует"}
	ErrChangeLeadRoleForbidden           = DefinedError{Code: 3007, StatusCode: http.StatusForbidden, Err: "change of lead role is forbidden", RuErr: "У вас недостаточно прав на изменение роли лидера проекта"}
	ErrCannotUpdateHigherRole            = DefinedError{Code: 3008, StatusCode: http.StatusForbidden, Err: "you cannot update a role that is higher than your own role", RuErr: "У вас недостаточно прав для изменения пользователя с более высокой ролью, чем ваша"}
	ErrCannotRemoveProjectLead           = DefinedError{Code: 3009, StatusCode: http.StatusBadRequest, Err: "cannot remove project lead from project", RuErr: "У вас недостаточно прав на удаление лидера проекта из проекта"}
	ErrCannotRemoveSelfFromProject       = DefinedError{Code: 3010, StatusCode: http.StatusBadRequest, Err: "you cannot remove yourself from the project", RuErr: "У вас недостаточно прав на удаление себя из проекта"}
	ErrCannotRemoveHigherRoleUserProject = DefinedError{Code: 3011, StatusCode: http.StatusForbidden, Err: "you cannot remove a user having a higher role than you", RuErr: "У вас недостаточно прав для удаления пользователя с более высокой ролью, чем ваша"}
	ErrRoleAndMemberIDRequired           = DefinedError{Code: 3012, StatusCode: http.StatusBadRequest, Err: "role and member ID are required", RuErr: "Необходимо указать роль и ID пользователя"}
	ErrUserNotInWorkspace                = DefinedError{Code: 3013, StatusCode: http.StatusBadRequest, Err: "user is not a member of the workspace. Invite the user to the workspace to add them to the project", RuErr: "Пользователь не является участником рабочего пространства. Необходимо добавить пользователя в рабочее пространство, а затем добавить его в проект"}
	ErrUserAlreadyInProject              = DefinedError{Code: 3014, StatusCode: http.StatusBadRequest, Err: "user is already a member of the project", RuErr: "Пользователь уже является участником проекта"}
	ErrProjectIsPrivate                  = DefinedError{Code: 3015, StatusCode: http.StatusForbidden, Err: "this is a private project %s", RuErr: "У вас недостаточно прав доступа к выбранному проекту"}
	ErrTagAlreadyExists                  = DefinedError{Code: 3016, StatusCode: http.StatusConflict, Err: "tag already exists in this project", RuErr: "Тег с таким именем и цветом уже создан в проекте"}
	ErrAlreadyImportingProject           = DefinedError{Code: 3017, StatusCode: http.StatusConflict, Err: "you are already importing another project", RuErr: "Вы уже импортируете другой проект"}
	ErrEstimatePointsRequired            = DefinedError{Code: 3018, StatusCode: http.StatusBadRequest, Err: "estimate points are required", RuErr: "Необходимо указать оценочные баллы"}
	ErrInvalidProjectViewProps           = DefinedError{Code: 3019, StatusCode: http.StatusBadRequest, Err: "invalid project view properties %s", RuErr: "Указаны некорректные параметры настроек проекта"}
	ErrProjectMemberNotFound             = DefinedError{Code: 3020, StatusCode: http.StatusBadRequest, Err: "project member not found", RuErr: "Участник проекта не найден"}
	ErrProjectNotFound                   = DefinedError{Code: 3021, StatusCode: http.StatusNotFound, Err: "project not found", RuErr: "Проект не найден"}
	ErrProjectLimitExceed                = DefinedError{Code: 3022, StatusCode: http.StatusPaymentRequired, Err: "project limit exceed", RuErr: "Количество ваших проектов достигло лимита бесплатной версии"}
	ErrProjectForbidden                  = DefinedError{Code: 3023, StatusCode: http.StatusForbidden, Err: "not have permissions to perform this action", RuErr: "Недостаточно прав для совершения действия"}
	ErrProjectStateNotFound              = DefinedError{Code: 3024, StatusCode: http.StatusBadRequest, Err: "state not found", RuErr: "Статус не найден"}
	ErrProjectStateInvalidSeqId          = DefinedError{Code: 3025, StatusCode: http.StatusBadRequest, Err: "invalid state SeqId", RuErr: "Неверный порядковый номер статуса в группе"}
	ErrProjectGroupNotFound              = DefinedError{Code: 3026, StatusCode: http.StatusNotFound, Err: "status group not found", RuErr: "Группа статусов не найдена"}
	ErrChangeProjectLead                 = DefinedError{Code: 3027, StatusCode: http.StatusBadRequest, Err: "error change of project lead", RuErr: "Не получилось изменить владельца пространства"}
	ErrIssueTemplateNotFound             = DefinedError{Code: 3028, StatusCode: http.StatusNotFound, Err: "issue template not found", RuErr: "Шаблон задачи не найден"}
	ErrIssueTemplateDuplicatedName       = DefinedError{Code: 3029, StatusCode: http.StatusConflict, Err: "issue template name already exist", RuErr: "Шаблон задачи с таким именем уже существует"}
	ErrAttachmentIsTooBig                = DefinedError{Code: 3030, StatusCode: http.StatusRequestEntityTooLarge, Err: "attachment size exceed 4GB size", RuErr: "Размер вложения не должен превышать 4ГБ"}

	// 32** - form errors
	ErrFormNotFound           = DefinedError{Code: 3201, StatusCode: http.StatusNotFound, Err: "form not found", RuErr: "Форма не найдена"}
	ErrFormAnswerForbidden    = DefinedError{Code: 3202, StatusCode: http.StatusForbidden, Err: "access to the form requires authorization", RuErr: "Для доступа к форме необходимо пройти авторизацию"}
	ErrFormForbidden          = DefinedError{Code: 3203, StatusCode: http.StatusForbidden, Err: "not allowed for current role", RuErr: "У вас недостаточно прав для выполнения действия"}
	ErrFormBadConvertRequest  = DefinedError{Code: 3204, StatusCode: http.StatusBadRequest, Err: "bad request, field: '%s'", RuErr: "При создании/обновлении формы передан неверный тип поля"}
	ErrFormBadRequest         = DefinedError{Code: 3205, StatusCode: http.StatusBadRequest, Err: "bad request", RuErr: "Некорректный запрос"}
	ErrFormRequestValidate    = DefinedError{Code: 3206, StatusCode: http.StatusBadRequest, Err: "validation error", RuErr: "Введены некорректные данные"}
	ErrFormCheckFields        = DefinedError{Code: 3207, StatusCode: http.StatusBadRequest, Err: "fields request error: '%s'", RuErr: "При создании формы задан неподдерживаемый тип поля"}
	ErrFormCheckAnswers       = DefinedError{Code: 3208, StatusCode: http.StatusBadRequest, Err: "required field missing or wrong type", RuErr: "При отправке ответа на форму не заполнены обязательные поля или выбран не соответствующий тип значения"}
	ErrFormEmptyAnswers       = DefinedError{Code: 3209, StatusCode: http.StatusBadRequest, Err: "empty answers", RuErr: "Для сохранения формы необходимо заполнить поля с ответами"}
	ErrFormAnswerNotFound     = DefinedError{Code: 3210, StatusCode: http.StatusNotFound, Err: "answer not found", RuErr: "Ответ не найден"}
	ErrFormAnswerEnd          = DefinedError{Code: 3211, StatusCode: http.StatusBadRequest, Err: "form is closed", RuErr: "Форма закрыта"}
	ErrLenAnswers             = DefinedError{Code: 3212, StatusCode: http.StatusBadRequest, Err: "incorrect number of answers", RuErr: "Некорректное количество ответов"}
	ErrFormEndDate            = DefinedError{Code: 3213, StatusCode: http.StatusBadRequest, Err: "the form cannot be created with a closed date", RuErr: "Форма не может быть создана с завершенной датой"}
	ErrFormAttachmentNotFound = DefinedError{Code: 3214, StatusCode: http.StatusBadRequest, Err: "file not found by the provided UUID", RuErr: "Файл по указанному UUID не найден"}
	ErrAttachmentInUse        = DefinedError{Code: 3215, StatusCode: http.StatusConflict, Err: "cannot delete file: it is linked to a form answer", RuErr: "Невозможно удалить файл — он привязан к ответу формы"}

	// 34** - doc errors
	ErrDocNotFound           = DefinedError{Code: 3401, StatusCode: http.StatusNotFound, Err: "doc not found", RuErr: "Документ не найден"}
	ErrDocUpdateForbidden    = DefinedError{Code: 3402, StatusCode: http.StatusForbidden, Err: "insufficient permissions or not the author", RuErr: "У вас недостаточно прав для изменения документа"}
	ErrDocDeleteHasChild     = DefinedError{Code: 3403, StatusCode: http.StatusForbidden, Err: "doc has child", RuErr: "Невозможно удалить, есть дочерние документы"}
	ErrDocBadRequest         = DefinedError{Code: 3404, StatusCode: http.StatusBadRequest, Err: "bad request", RuErr: "Некорректный запрос"}
	ErrDocCommentNotFound    = DefinedError{Code: 3405, StatusCode: http.StatusNotFound, Err: "doc comment not found", RuErr: "Не найден комментарий"}
	ErrDocRequestValidate    = DefinedError{Code: 3406, StatusCode: http.StatusBadRequest, Err: "validation error", RuErr: "Введены некорректные данные"}
	ErrDocForbidden          = DefinedError{Code: 3407, StatusCode: http.StatusForbidden, Err: "not have permissions to perform this action", RuErr: "Недостаточно прав для совершения действия"}
	ErrDocCommentBadRequest  = DefinedError{Code: 3408, StatusCode: http.StatusBadRequest, Err: "bad request", RuErr: "Некорректный запрос"}
	ErrDocAttachmentNotFound = DefinedError{Code: 3409, StatusCode: http.StatusNotFound, Err: "doc attachment not found", RuErr: "Не найдено вложение"}
	ErrDocOrderBadRequest    = DefinedError{Code: 3410, StatusCode: http.StatusBadRequest, Err: "there is a document sequence error in the request", RuErr: "ошибка последовательности документов"}
	ErrDocChildRoleTooLow    = DefinedError{Code: 3411, StatusCode: http.StatusBadRequest, Err: "child doc role must not be lower than parent's", RuErr: "Роль дочернего документа не может быть ниже родительского"}
	ErrDocParentRoleTooLow   = DefinedError{Code: 3412, StatusCode: http.StatusBadRequest, Err: "parent doc role must not be lower than any child", RuErr: "Роль родительского документа не может быть ниже дочернего"}
	ErrDocMoveIntoOwnChild   = DefinedError{Code: 3413, StatusCode: http.StatusBadRequest, Err: "cannot move document into its own child", RuErr: "Невозможно переместить документ в его же дочерний"}
	ErrDocCommentEmpty       = DefinedError{Code: 3414, StatusCode: http.StatusBadRequest, Err: "comment is empty", RuErr: "Попытка отправить пустой комментарий"}

	// 36** - sprint errors
	ErrSprintNotFound          = DefinedError{Code: 3601, StatusCode: http.StatusNotFound, Err: "sprint not found", RuErr: "Спринт не найден"}
	ErrSprintUpdateForbidden   = DefinedError{Code: 3602, StatusCode: http.StatusForbidden, Err: "insufficient permissions or not the author", RuErr: "У вас недостаточно прав для внесения изменения в спринт"}
	ErrSprintForbidden         = DefinedError{Code: 3603, StatusCode: http.StatusForbidden, Err: "not have permissions to perform this action", RuErr: "Недостаточно прав для совершения действия"}
	ErrSprintBadRequest        = DefinedError{Code: 3604, StatusCode: http.StatusBadRequest, Err: "bad request", RuErr: "Некорректный запрос"}
	ErrSprintRequestValidate   = DefinedError{Code: 3605, StatusCode: http.StatusBadRequest, Err: "validation error", RuErr: "Введены некорректные данные"}
	ErrInvalidSprintViewProps  = DefinedError{Code: 3606, StatusCode: http.StatusBadRequest, Err: "invalid sprint view properties %s", RuErr: "Указаны некорректные параметры настроек спринта"}
	ErrInvalidSprintTimeWindow = DefinedError{Code: 3607, StatusCode: http.StatusBadRequest, Err: "invalid sprint time window", RuErr: "Некорректный период спринта"}
	// 4*** - issue errors
	ErrIssueNotFound                   = DefinedError{Code: 4001, StatusCode: http.StatusNotFound, Err: "issue not found", RuErr: "Задача не найдена"}
	ErrDeleteIssueForbidden            = DefinedError{Code: 4002, StatusCode: http.StatusForbidden, Err: "only admin and author can delete issue", RuErr: "У вас недостаточно прав для удаления задачи"}
	ErrStateAlreadyExists              = DefinedError{Code: 4004, StatusCode: http.StatusConflict, Err: "this state already exists", RuErr: "Статус с таким именем и цветом уже создан в проекте"}
	ErrDefaultStateCannotBeDeleted     = DefinedError{Code: 4005, StatusCode: http.StatusBadRequest, Err: "default state cannot be deleted", RuErr: "Удаление статуса по умолчанию невозможно"}
	ErrStateNotEmptyCannotDelete       = DefinedError{Code: 4006, StatusCode: http.StatusBadRequest, Err: "the state is not empty, only empty states can be deleted", RuErr: "Удаление статуса, установленного для задачи, невозможно"}
	ErrCommentEditForbidden            = DefinedError{Code: 4007, StatusCode: http.StatusForbidden, Err: "insufficient privileges or not your comment", RuErr: "У вас недостаточно прав. Удаление или изменение комментария доступно только автору комментария"}
	ErrIssueUpdateForbidden            = DefinedError{Code: 4008, StatusCode: http.StatusForbidden, Err: "insufficient permissions or not the author", RuErr: "У вас недостаточно прав для изменения задачи"}
	ErrInvalidReaction                 = DefinedError{Code: 4009, StatusCode: http.StatusBadRequest, Err: "invalid reaction", RuErr: "Выбрана недопустимая реакция"}
	ErrTooManyComments                 = DefinedError{Code: 4010, StatusCode: http.StatusTooManyRequests, Err: "too many comments creation requests", RuErr: "Попытка отправить несколько комментариев подряд"}
	ErrIssueLimitExceed                = DefinedError{Code: 4011, StatusCode: http.StatusPaymentRequired, Err: "issue limit exceed", RuErr: "Количество ваших задач достигло лимита бесплатной версии"}
	ErrAssetsLimitExceed               = DefinedError{Code: 4012, StatusCode: http.StatusPaymentRequired, Err: "attachment limit exceed", RuErr: "Количество ваших вложений достигло лимита вашего плана"}
	ErrIssueNameEmpty                  = DefinedError{Code: 4013, StatusCode: http.StatusBadRequest, Err: "Empty issue name", RuErr: "Передано пустое имя задачи"}
	ErrPermissionParentIssue           = DefinedError{Code: 4081, StatusCode: http.StatusConflict, Err: "the task was not created by the current user", RuErr: "Выбранная задача не может быть преобразована в подзадачу. Выбранная задача не вашего авторства"}
	ErrIssueForbidden                  = DefinedError{Code: 4014, StatusCode: http.StatusForbidden, Err: "not have permissions to perform this action", RuErr: "Недостаточно прав для совершения действия"}
	ErrIssueCommentNotFound            = DefinedError{Code: 4015, StatusCode: http.StatusNotFound, Err: "issue comment not found", RuErr: "Комментарий не найден"}
	ErrTooHeavyAttachmentsZip          = DefinedError{Code: 4016, StatusCode: http.StatusRequestEntityTooLarge, Err: "attachments size exceed " + fmt.Sprint(AttachmentsZipMaxSizeMB) + "MB", RuErr: "Суммарный размер вложений превышает " + fmt.Sprint(AttachmentsZipMaxSizeMB) + "МБ"}
	ErrAttachmentsIncorrectMetadata    = DefinedError{Code: 4017, StatusCode: http.StatusBadRequest, Err: "incorrect attachment metadata", RuErr: "Ошибка клиента"}
	ErrChildDependency                 = DefinedError{Code: 4018, StatusCode: http.StatusBadRequest, Err: "cyclic dependency detected", RuErr: "Попытка создания циклической зависимости дочерних задач"}
	ErrIssueDescriptionLocked          = DefinedError{Code: 4019, StatusCode: http.StatusTooManyRequests, Err: "issue description locked by another user", RuErr: "Задача редактируется другим пользователем"}
	ErrIssueDescriptionNotLockedByUser = DefinedError{Code: 4020, StatusCode: http.StatusForbidden, Err: "you are not locking this issue description", RuErr: "Вы не редактируете сейчас эту задачу"}
	ErrIssueScriptFail                 = DefinedError{Code: 4021, StatusCode: http.StatusForbidden, Err: "prohibition of committing an action", RuErr: "Запрет на совершение действия"}
	ErrIssueCustomScriptFail           = DefinedError{Code: 4022, StatusCode: http.StatusForbidden, Err: "prohibition of committing an action", RuErr: "Запрет на совершение действия"}
	ErrUnsupportedGroup                = DefinedError{Code: 4023, StatusCode: http.StatusBadRequest, Err: "unsupported grouping param", RuErr: "Данный параметр не поддерживается для группировки"}
	ErrIssueTargetDateExp              = DefinedError{Code: 4024, StatusCode: http.StatusBadRequest, Err: "the date has already passed", RuErr: "Заданная дата уже прошла"}
	ErrIssueCommentEmpty               = DefinedError{Code: 4025, StatusCode: http.StatusBadRequest, Err: "comment is empty", RuErr: "Попытка отправить пустой комментарий"}

	// 5*** - validation and other errors
	ErrInvalidEmail         = DefinedError{Code: 5001, StatusCode: http.StatusBadRequest, Err: "invalid email %s", RuErr: "Указан некорректный email"}
	ErrLimitTooHigh         = DefinedError{Code: 5002, StatusCode: http.StatusBadRequest, Err: "limit must be less than 100", RuErr: "Запрашиваемый список задач должен состоящий не более чем из 100 элементов"}
	ErrUnsupportedSortParam = DefinedError{Code: 5003, StatusCode: http.StatusBadRequest, Err: "unsupported sort parameter %s", RuErr: "Неподдерживаемый параметр сортировки"}
	ErrURLAndTitleRequired  = DefinedError{Code: 5004, StatusCode: http.StatusBadRequest, Err: "URL and Title are required", RuErr: "Необходимо ввести URL и заголовок"}
	ErrIssueIDsRequired     = DefinedError{Code: 5005, StatusCode: http.StatusBadRequest, Err: "issue IDs are required", RuErr: "Необходимо ввести ID задачи"}
	ErrUnsupportedRole      = DefinedError{Code: 5006, StatusCode: http.StatusBadRequest, Err: "unsupported role %s", RuErr: "При добавлении пользователя в пространство указана некорректная роль"}
	ErrDeleteSuperUser      = DefinedError{Code: 5007, StatusCode: http.StatusBadRequest, Err: "you cannot delete superuser", RuErr: "У вас недостаточно прав на удаление суперпользователя"}
	ErrGeneric              = DefinedError{Code: 5000, StatusCode: http.StatusBadRequest, Err: "Something went wrong. Please try again later or contact the support team.", RuErr: "Что-то пошло не так. Повторите попытку позже или обратитесь в службу поддержки"}
	ErrInvalidDayFormat     = DefinedError{Code: 5008, StatusCode: http.StatusBadRequest, Err: "day must be in ddmmyyyy format", RuErr: "Необходимо указать дату в формате: ддммгггг"}
	ErrDemo                 = DefinedError{Code: 5009, StatusCode: http.StatusPaymentRequired, Err: "forbidden action in demo mode", RuErr: "Данное действие недоступно в демо-режиме"}
	ErrEntityToLarge        = DefinedError{Code: 5010, StatusCode: http.StatusRequestEntityTooLarge, Err: "size exceeds the allowed limit", RuErr: "Размер файла превышает допустимый."}
	ErrDuplicateEmail       = DefinedError{Code: 5011, StatusCode: http.StatusBadRequest, Err: "duplicated email", RuErr: "Пользователь с таким email уже добавлен"}
	ErrFileTooLarge         = DefinedError{Code: 5012, StatusCode: http.StatusRequestEntityTooLarge, Err: "uploaded file exceeds the 50MB size limit", RuErr: "Загруженный файл превышает допустимый размер 50 МБ"}

	// 6*** - user errors
	ErrUserNotFound             = DefinedError{Code: 6001, StatusCode: http.StatusNotFound, Err: "user not found", RuErr: "Пользователь не найден"}
	ErrUsernameConflict         = DefinedError{Code: 6002, StatusCode: http.StatusConflict, Err: "username already exists", RuErr: "Пользователь с таким именем уже зарегистрирован в системе"}
	ErrNameRequired             = DefinedError{Code: 6003, StatusCode: http.StatusBadRequest, Err: "first_name and last_name are required", RuErr: "Поля имя и фамилия не могут быть пустыми"}
	ErrPasswordsNotEqual        = DefinedError{Code: 6004, StatusCode: http.StatusBadRequest, Err: "passwords do not match", RuErr: "Пароли не совпадают"}
	ErrInvalidResetToken        = DefinedError{Code: 6005, StatusCode: http.StatusNotFound, Err: "invalid reset token or user ID", RuErr: "Неверный токен сброса или ID пользователя"}
	ErrChangePasswordForbidden  = DefinedError{Code: 6006, StatusCode: http.StatusForbidden, Err: "only superuser can change password of another user", RuErr: "У вас недостаточно прав на смену пароля другого пользователя"}
	ErrNotOwnFilter             = DefinedError{Code: 6007, StatusCode: http.StatusForbidden, Err: "you do not own this filter", RuErr: "В вашем списке нет такого фильтра"}
	ErrCannotAddNonPublicFilter = DefinedError{Code: 6008, StatusCode: http.StatusForbidden, Err: "you cannot add a non-public filter", RuErr: "У вас недостаточно прав на сохранение приватного фильтра в свой список"}
	ErrCannotRemoveOwnFilter    = DefinedError{Code: 6009, StatusCode: http.StatusForbidden, Err: "you cannot remove your own filter", RuErr: "У вас недостаточно прав на удаление фильтра"}
	ErrBadNotifyIds             = DefinedError{Code: 6010, StatusCode: http.StatusBadRequest, Err: "bad ids request", RuErr: "Не верный список идентификаторов"}
	ErrUnsupportedAvatarType    = DefinedError{Code: 6011, StatusCode: http.StatusUnsupportedMediaType, Err: "unsupported avatar file type", RuErr: "Данный тип файла не поддерживается для установки аватара"}
	ErrFilterBadRequest         = DefinedError{Code: 6012, StatusCode: http.StatusBadRequest, Err: "incorrect format of transmitted data", RuErr: "Не верный формат переданных данных"}
	ErrBadTimezone              = DefinedError{Code: 6013, StatusCode: http.StatusBadRequest, Err: "wrong timezone ", RuErr: "Временная зона не поддерживается"}
	ErrEmailIsExist             = DefinedError{Code: 6014, StatusCode: http.StatusBadRequest, Err: "email is exist", RuErr: "Пользователь с таким Email уже существует"}
	ErrEmailChangeLimit         = DefinedError{Code: 6015, StatusCode: http.StatusTooManyRequests, Err: "the code was sent less than a minute ago", RuErr: "Запрос нового кода верификации можно делать раз в минуту"}
	ErrEmailVerify              = DefinedError{Code: 6016, StatusCode: http.StatusBadRequest, Err: "invalid or expired code", RuErr: "Неверный или просроченный код"}

	// 7*** - integration errors
	ErrInvalidEventType      = DefinedError{Code: 7001, StatusCode: http.StatusBadRequest, Err: "invalid event type", RuErr: "Указан неверный тип события"}
	ErrCannotEditIntegration = DefinedError{Code: 7002, StatusCode: http.StatusBadRequest, Err: "cannot edit integration", RuErr: "Невозможно редактировать интеграцию"}

	// 8*** - import errors
	ErrImportIDRequired       = DefinedError{Code: 8001, StatusCode: http.StatusBadRequest, Err: "importId must be specified", RuErr: "Необходимо указать ID импорта"}
	ErrJiraInvalidCredentials = DefinedError{Code: 8002, StatusCode: http.StatusBadRequest, Err: "jira invalid credentials", RuErr: "Ошибка аутентификации: проверьте логин, пароль или API-ключ для подключения к Jira"}

	// 9*** - admin errors
	ErrReleaseNoteNotFound   = DefinedError{Code: 9001, StatusCode: http.StatusNotFound, Err: "release note not found", RuErr: "Изменение версии не найдено"}
	ErrInvalidCronExpression = DefinedError{Code: 9002, StatusCode: http.StatusBadRequest, Err: "invalid cron expression", RuErr: "Неверное выражение для периодического задания"}
	ErrInvalidID             = DefinedError{Code: 9003, StatusCode: http.StatusBadRequest, Err: "invalid ID", RuErr: "Указан неверный ID"}
	ErrCronSettingNotFound   = DefinedError{Code: 9004, StatusCode: http.StatusNotFound, Err: "cron setting not found", RuErr: "Настройки периодического задания не найдены"}
	ErrReleaseNoteExists     = DefinedError{Code: 9005, StatusCode: http.StatusConflict, Err: "release note for this version already exists", RuErr: "Заметка к текущему релизу уже существует"}
	ErrTariffExist           = DefinedError{Code: 9006, StatusCode: http.StatusConflict, Err: "tariff for this user already exists", RuErr: "Данный пользователь уже тарифицируется"}
	ErrTariffNotFound        = DefinedError{Code: 9007, StatusCode: http.StatusNotFound, Err: "tariff not found", RuErr: "Тариф не найден"}
	ErrReleaseNoteEmptyBody  = DefinedError{Code: 9008, StatusCode: http.StatusBadRequest, Err: "the note content cannot be empty", RuErr: "Тело заметки к релизу не может быть пустым"}

	// 10*** - git errors
	ErrGitDisabled              = DefinedError{Code: 10001, StatusCode: http.StatusForbidden, Err: "git functionality is disabled", RuErr: "Функциональность Git отключена"}
	ErrGitRepositoryExists      = DefinedError{Code: 10002, StatusCode: http.StatusConflict, Err: "repository with this name already exists", RuErr: "Репозиторий с таким именем уже существует"}
	ErrGitInvalidRepositoryName = DefinedError{Code: 10003, StatusCode: http.StatusBadRequest, Err: "invalid repository name: only alphanumeric characters, hyphens, underscores and dots are allowed", RuErr: "Некорректное имя репозитория: допустимы только буквы, цифры, дефисы, подчеркивания и точки"}
	ErrGitRepositoryNotFound    = DefinedError{Code: 10004, StatusCode: http.StatusNotFound, Err: "git repository not found", RuErr: "Git репозиторий не найден"}
	ErrGitCommandFailed         = DefinedError{Code: 10005, StatusCode: http.StatusInternalServerError, Err: "git command failed: %s", RuErr: "Не удалось выполнить git команду: %s"}
	ErrGitInvalidBranch         = DefinedError{Code: 10006, StatusCode: http.StatusBadRequest, Err: "invalid branch name", RuErr: "Некорректное имя ветки"}
	ErrGitPathCreationFailed    = DefinedError{Code: 10007, StatusCode: http.StatusInternalServerError, Err: "failed to create repository directory", RuErr: "Не удалось создать директорию для репозитория"}

	// 11*** - SSH errors
	ErrSSHKeyInvalidData    = DefinedError{Code: 11001, StatusCode: http.StatusBadRequest, Err: "invalid SSH key data", RuErr: "Некорректные данные SSH ключа"}
	ErrSSHKeyAlreadyExists  = DefinedError{Code: 11002, StatusCode: http.StatusConflict, Err: "SSH key with this fingerprint already exists", RuErr: "SSH ключ с таким отпечатком уже существует"}
	ErrSSHKeyNotFound       = DefinedError{Code: 11003, StatusCode: http.StatusNotFound, Err: "SSH key not found", RuErr: "SSH ключ не найден"}
	ErrSSHInvalidPublicKey  = DefinedError{Code: 11004, StatusCode: http.StatusBadRequest, Err: "invalid SSH public key format", RuErr: "Неверный формат SSH публичного ключа"}
	ErrSSHAccessDenied      = DefinedError{Code: 11005, StatusCode: http.StatusForbidden, Err: "SSH access denied", RuErr: "SSH доступ запрещен"}
	ErrSSHDisabled          = DefinedError{Code: 11006, StatusCode: http.StatusForbidden, Err: "SSH access is disabled", RuErr: "SSH доступ отключен"}
	ErrSSHRateLimitExceeded = DefinedError{Code: 11007, StatusCode: http.StatusTooManyRequests, Err: "SSH rate limit exceeded", RuErr: "Превышен лимит SSH запросов"}
)

func (e DefinedError) WithFormattedMessage(args ...interface{}) DefinedError {
	if len(args) > 0 {
		e.Err = fmt.Sprintf(e.Err, args...)
		e.RuErr = fmt.Sprintf(e.RuErr, args...)
	} else {
		e.Err = strings.Replace(e.Err, "%s", "", -1)
		e.RuErr = strings.Replace(e.RuErr, "%s", "", -1)
	}
	return e
}
