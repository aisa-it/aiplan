// Пакет server предоставляет основные компоненты для управления планированием и задачами. Он включает в себя функциональность для работы с активностями, пользователями, проектами, рабочими местами и другими связанными данными. Также предоставляет API для интеграции с другими сервисами и внешними системами.
//
// Основные возможности:
//   - Управление активностями пользователей.
//   - Работа с проектами и рабочими местами.
//   - Интеграция с внешними сервисами (например, Telegram, email).
//   - Генерация и обработка аватаров пользователей.
//   - Поддержка различных типов данных и форматов.
package server

// @title AIPlan API
// @version 1.0
// @description AIPlan - open-source project management system with task management, document collaboration, forms, video conferencing, and calendar features.
// @securityDefinitions.apikey ApiKeyAuth
// @in header
// @name Authorization
// @BasePath /
// @query.collection.format multi
// @tag.name GIT
// @tag.description Git repository integration endpoints
import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"io"
	"net/http"
	"net/mail"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/pkg/notifications/email"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/token"

	"github.com/aisa-it/aiplan/aiplan.go/pkg/types"

	"github.com/nfnt/resize"

	"image/jpeg"
	_ "image/png"

	"github.com/aisa-it/aiplan/aiplan.go/pkg/config"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/dao"
	"github.com/gofrs/uuid"
	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/pbkdf2"
	"gorm.io/gorm"

	_ "github.com/aisa-it/aiplan/aiplan.go/pkg/server/docs"
)

//go:generate go run github.com/swaggo/swag/cmd/swag@latest --version
//go:generate go run github.com/swaggo/swag/cmd/swag@latest init -ot go,json --generalInfo /http.go --propertyStrategy snakecase --dir ./ --output docs --parseDependency 1
//go:generate echo "Generate docs"
//go:generate go run ../../cmd/docsgen/main.go -src ../apierrors/apierrors.go -out ../../../aiplan-help/api_errors.md
//go:generate echo "Generate schema"
//go:generate go run ../../cmd/schemagen/main.go

const requestTimeout = time.Second * 20

const spanCtxKey = "trace.span_context"

// Переходный костыль: пакетные конфиг и версия, которые ещё читают обработчики
// пакета. Заполняются в NewServices; публичный API их не показывает
// (см. Services.Config/Version). Пока они есть — один инстанс сервера на процесс.
var cfg *config.Config
var appVersion string

// Проверка email на корректность
func ValidateEmail(email string) bool {
	_, err := mail.ParseAddress(email)
	return err == nil
}

// Проверка хешированого пароля
func checkPassword(password string, pass string) bool {
	ss := strings.Split(pass, "$")
	if len(ss) == 4 {
		if base64.StdEncoding.EncodeToString(pbkdf2.Key([]byte(password), []byte(ss[2]), 260000, 32, sha256.New)) == ss[3] {
			return true
		} else {
			return false
		}
	}

	return false
}

// Генерация ключа доступа
func createAccessToken(userId uuid.UUID) (*token.Token, *token.Token, error) {
	ta, err := token.GenJwtToken([]byte(cfg.SecretKey), "access", userId)
	if err != nil {
		return nil, nil, err
	}

	tr, err := token.GenJwtToken([]byte(cfg.SecretKey), "refresh", userId)
	if err != nil {
		return nil, nil, err
	}
	return ta, tr, err
}

func setAuthCookies(c echo.Context, accessToken *token.Token, refreshToken *token.Token) {
	accessCookie := new(http.Cookie)
	accessCookie.Name = "access_token"
	accessCookie.Value = accessToken.SignedString
	accessCookie.HttpOnly = true
	accessCookie.Secure = cfg.WebURL.URL.Scheme == "https"
	accessCookie.Path = "/"
	accessCookie.SameSite = http.SameSiteNoneMode
	if cfg.WebURL.URL.Scheme != "https" {
		accessCookie.SameSite = http.SameSiteLaxMode
	}
	accessCookie.Expires = time.Now().Add(types.TokenExpiresPeriod)
	c.SetCookie(accessCookie)

	refreshCookie := new(http.Cookie)
	refreshCookie.Name = "refresh_token"
	refreshCookie.Value = refreshToken.SignedString
	refreshCookie.HttpOnly = true
	refreshCookie.Secure = cfg.WebURL.URL.Scheme == "https"
	refreshCookie.Path = "/"
	refreshCookie.SameSite = http.SameSiteNoneMode
	if cfg.WebURL.URL.Scheme != "https" {
		refreshCookie.SameSite = http.SameSiteLaxMode
	}
	refreshCookie.Expires = time.Now().Add(types.RefreshTokenExpiresPeriod)
	c.SetCookie(refreshCookie)
}

func clearAuthCookies(c echo.Context) {
	accessCookie := new(http.Cookie)
	accessCookie.Name = "access_token"
	accessCookie.Value = ""
	accessCookie.HttpOnly = true
	accessCookie.Secure = cfg.WebURL.URL.Scheme == "https"
	accessCookie.Path = "/"
	accessCookie.SameSite = http.SameSiteNoneMode
	if cfg.WebURL.URL.Scheme != "https" {
		accessCookie.SameSite = http.SameSiteLaxMode
	}
	accessCookie.MaxAge = -1
	c.SetCookie(accessCookie)

	refreshCookie := new(http.Cookie)
	refreshCookie.Name = "refresh_token"
	refreshCookie.Value = ""
	refreshCookie.HttpOnly = true
	refreshCookie.Secure = cfg.WebURL.URL.Scheme == "https"
	refreshCookie.Path = "/"
	refreshCookie.SameSite = http.SameSiteNoneMode
	if cfg.WebURL.URL.Scheme != "https" {
		refreshCookie.SameSite = http.SameSiteLaxMode
	}
	refreshCookie.MaxAge = -1
	c.SetCookie(refreshCookie)
}

func GenInviteToken(email string) (string, error) {
	claim := jwt.MapClaims{
		"email":     email,
		"timestamp": time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claim)
	ret, err := token.SignedString([]byte(cfg.SecretKey))
	return ret, err
}

func GenTokenChangeMail(email string) (string, error) {
	claim := jwt.MapClaims{
		"exp":   jwt.NewNumericDate(time.Now().Add(types.EmailCodeLifeTime)),
		"iat":   jwt.NewNumericDate(time.Now()),
		"email": email,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claim)
	ret, err := token.SignedString([]byte(cfg.SecretKey))
	return ret, err
}

func StructToJSONMap(obj interface{}) map[string]interface{} {
	val := reflect.ValueOf(obj)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}
	if val.Kind() != reflect.Struct {
		return nil
	}

	res := make(map[string]interface{})
	typ := val.Type()

	for i := 0; i < val.NumField(); i++ {
		field := typ.Field(i)
		fieldValue := val.Field(i)

		if !field.IsExported() {
			continue
		}

		tag := field.Tag.Get("json")
		if tag == "-" {
			continue
		}

		tagParts := strings.Split(tag, ",")
		tagName := tagParts[0]
		if tagName == "" {
			tagName = field.Name
		}
		tagOptions := tagParts[1:]

		omitEmpty := false
		for _, option := range tagOptions {
			if option == "omitempty" {
				omitEmpty = true
				break
			}
		}
		if omitEmpty && fieldValue.Kind() != reflect.Bool && isNilOrZero(fieldValue) {
			continue
		}
		var result interface{}
		if fieldValue.CanInterface() {
			if fieldValue.Kind() == reflect.Ptr && !fieldValue.IsNil() {
				if fieldValue.Elem().Type() == reflect.TypeOf(time.Time{}) {
					fieldValue = fieldValue.Elem()
				}
			}
			if fieldValue.Type() == reflect.TypeOf(uuid.UUID{}) {
				res[tagName] = fieldValue.Interface().(uuid.UUID)
				continue
			}
			if fieldValue.Type() == reflect.TypeOf(uuid.NullUUID{}) {
				res[tagName] = fieldValue.Interface().(uuid.NullUUID)
				continue
			}
			if fieldValue.Type() == reflect.TypeOf(time.Time{}) {
				res[tagName] = fieldValue.Interface().(time.Time)
				continue
			}
			if fieldValue.Type() == reflect.TypeOf(&types.TargetDate{}) {
				res[tagName] = fieldValue.Interface().(*types.TargetDate)
				continue
			}
			if fieldValue.Type() == reflect.TypeOf(types.RedactorHTML{}) {
				res[tagName] = fieldValue.Interface().(types.RedactorHTML)
				continue
			}
			if fieldValue.Type() == reflect.TypeOf(types.FormAnswerNotify{}) {
				res[tagName] = fieldValue.Interface().(types.FormAnswerNotify)
				continue
			}
			if marshaler, ok := fieldValue.Interface().(json.Marshaler); ok {
				if fieldValue.Kind() == reflect.Ptr && fieldValue.IsNil() {
					res[tagName] = nil
					continue
				}
				jsonValue, err := marshaler.MarshalJSON()
				if err != nil {
					continue
				}
				if string(jsonValue) == "null" {
					res[tagName] = nil
				} else {
					res[tagName] = strings.Trim(string(jsonValue), "\"")
				}
				continue
			}
		}

		switch fieldValue.Kind() {
		case reflect.Slice:
			if fieldValue.IsNil() {
				result = nil
			} else {
				slice := make([]interface{}, fieldValue.Len())
				for i := 0; i < fieldValue.Len(); i++ {
					item := fieldValue.Index(i).Interface()
					if reflect.TypeOf(item).Kind() == reflect.Struct {
						slice[i] = StructToJSONMap(item)
					} else {
						slice[i] = item
					}
				}
				result = slice
			}
		case reflect.Struct:
			if fieldValue.Type() == reflect.TypeOf(time.Time{}) {
				result = fieldValue.Interface().(time.Time)
			} else {
				if fieldValue.Kind() == reflect.Ptr {
					result = StructToJSONMap(fieldValue.Elem().Interface())
				} else {
					result = StructToJSONMap(fieldValue.Interface())
				}
			}
		case reflect.Ptr, reflect.Interface:
			if fieldValue.IsNil() {
				result = nil
			} else {
				if fieldValue.Kind() == reflect.Ptr && fieldValue.Elem().Kind() == reflect.String {
					result = fieldValue.Elem().Interface().(string)
				} else if fieldValue.Kind() == reflect.Ptr {
					result = fieldValue.Elem().Interface()
				} else {
					result = fieldValue.Interface()
				}
			}
		default:
			result = fieldValue.Interface()
		}
		res[tagName] = result
	}
	return res
}

func isNilOrZero(value reflect.Value) bool {
	if !value.IsValid() {
		return true
	}
	if value.Kind() == reflect.Ptr || value.Kind() == reflect.Interface {
		return value.IsNil()
	}
	return reflect.DeepEqual(value.Interface(), reflect.Zero(value.Type()).Interface())
}

func IsValidRole(role int) bool {
	switch role {
	case
		5,
		10,
		15:
		return true
	}
	return false
}

func CheckWorkspaceSlug(slug string) bool {
	return !slices.Contains([]string{
		"api",
		"create-workspace",
		"error",
		"installations",
		"invitations",
		"magic-sign-in",
		"onboarding",
		"reset-password",
		"signin",
		"signup",
		"workspace-member-invitation",
		"404",
		"undefined",
		"no-workspace",
		"profile",
		"not-found",
		"forms",
		"swagger",
		"filters",
		"sf",
	}, slug)
}

func imageThumbnail(r io.Reader, contentType string) (io.Reader, int, string, error) {
	var err error
	dataType := "image/jpeg"

	buf := new(bytes.Buffer)
	switch contentType {
	case "image/gif":
		/* Maybe resize gifs in future
		   var g *gif.GIF
		   g, err = gif.DecodeAll(r)
		   if err != nil {
		   	return nil, 0, "", err
		   }

		   newGif := gif.GIF{}

		   for i, frame := range g.Image {
		   	resizedFrame := resize.Thumbnail(512, 512, frame, resize.Lanczos3)
		   	if resizedFrame.Bounds().Max.X > 512 || resizedFrame.Bounds().Min.Y > 512 {
		   		continue
		   	}
		   	palettedImg := image.NewPaletted(resizedFrame.Bounds(), frame.Palette)
		   	draw.FloydSteinberg.Draw(palettedImg, resizedFrame.Bounds(), resizedFrame, resizedFrame.Bounds().Min)

		   	newGif.Image = append(newGif.Image, palettedImg)
		   	newGif.Delay = append(newGif.Delay, g.Delay[i])
		   }

		   err = gif.EncodeAll(buf, &newGif)*/
		io.Copy(buf, r)
		dataType = "image/gif"
	default:
		var img image.Image
		img, _, err = image.Decode(r)
		if err != nil {
			return nil, 0, "", err
		}
		thmb := resize.Thumbnail(512, 512, img, resize.Lanczos3)
		err = jpeg.Encode(buf, thmb, &jpeg.Options{Quality: 80})
	}
	return buf, buf.Len(), dataType, err
}

func GetActivitiesTable(query *gorm.DB, from DayRequest, to DayRequest) (map[uuid.UUID]types.ActivityTable, error) {
	var activities []struct {
		ActorId uuid.UUID
		Day     time.Time
		Cnt     int
	}
	if err := query.
		Select("actor_id, date_trunc('day', created_at) as Day, count(*) as Cnt").
		Where("created_at between ? and ?", time.Time(from), time.Time(to)).
		Where("actor_id is not null").
		Group("actor_id, Day").
		Order("Day").
		Model(&dao.ActivityEvent{}).
		Find(&activities).Error; err != nil {
		return nil, err
	}

	resp := make(map[uuid.UUID]types.ActivityTable)
	for _, activity := range activities {
		m, ok := resp[activity.ActorId]
		if !ok {
			m = make(types.ActivityTable)
		}
		m[types.Day(activity.Day)] = types.ActivityTableDay{
			Weekday: types.WeekdayShort(activity.Day.Weekday()),
			Count:   activity.Cnt,
		}
		resp[activity.ActorId] = m
	}
	return resp, nil
}

func BindData(c echo.Context, key string, target interface{}) ([]string, error) {
	var fields []string
	form, _ := c.MultipartForm()

	if key != "" && form != nil {
		formValue := c.FormValue(key)
		if formValue != "" {
			if err := json.Unmarshal([]byte(formValue), target); err != nil {
				return nil, fmt.Errorf("failed to unmarshal data from FormValue[%s]: %w", key, err)
			}
		}
	} else {
		if err := c.Bind(target); err != nil {
			return nil, fmt.Errorf("failed to bind data from JSON body: %w", err)
		}
	}

	rawMap := StructToJSONMap(target)
	for keyRaw := range rawMap {
		fields = append(fields, keyRaw)
	}
	return fields, nil
}

func CompareAndAddFields(f1, f2 interface{}, name string, fields *[]string) error {
	if f1 == nil || f2 == nil {
		return fmt.Errorf("one of the values is nil")
	}

	valF1 := reflect.ValueOf(f1)
	valF2 := reflect.ValueOf(f2)

	if valF1.Kind() != reflect.Ptr || valF2.Kind() != reflect.Ptr {
		return fmt.Errorf("both parameters must be pointers, got %T and %T", f1, f2)
	}

	elemF1 := valF1.Elem()
	elemF2 := valF2.Elem()

	if elemF1.Type() != elemF2.Type() {
		return fmt.Errorf("types do not match: %s and %s", elemF1.Type(), elemF2.Type())
	}
	if elemF1.Kind() == reflect.Slice {
		if !slicesEqualIgnoreOrder(elemF1, elemF2) {
			elemF1.Set(elemF2)
			*fields = append(*fields, name)
		}
		return nil
	}

	if !reflect.DeepEqual(elemF1.Interface(), elemF2.Interface()) {
		elemF1.Set(elemF2)
		*fields = append(*fields, name)
	}

	return nil
}

func slicesEqualIgnoreOrder(s1, s2 reflect.Value) bool {
	if s1.Len() != s2.Len() {
		return false
	}

	counts := make(map[interface{}]int)

	for i := 0; i < s1.Len(); i++ {
		val := s1.Index(i).Interface()
		counts[val]++
	}

	for i := 0; i < s2.Len(); i++ {
		val := s2.Index(i).Interface()
		if counts[val] == 0 {
			return false
		}
		counts[val]--
	}

	for _, v := range counts {
		if v != 0 {
			return false
		}
	}

	return true
}

func sendPasswordDefaultAdmin(tx *gorm.DB, es *email.EmailService) {
	var user dao.User
	if err := tx.Where("username = ?", "admin").Where("is_onboarded = ?", false).First(&user).Error; err != nil {
		return
	}

	err := es.NewUserPasswordNotify(user, "password123")
	if err != nil {
		return
	}
}
