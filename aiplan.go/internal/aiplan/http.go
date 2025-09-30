// Пакет aiplan предоставляет основные компоненты для управления планированием и задачами. Он включает в себя функциональность для работы с активностями, пользователями, проектами, рабочими местами и другими связанными данными. Также предоставляет API для интеграции с другими сервисами и внешними системами.
//
// Основные возможности:
//   - Управление активностями пользователей.
//   - Работа с проектами и рабочими местами.
//   - Интеграция с внешними сервисами (например, Telegram, email).
//   - Генерация и обработка аватаров пользователей.
//   - Поддержка различных типов данных и форматов.
package aiplan

// @title My API
// @version 1.0
// @description This is a sample server.
// @securityDefinitions.apikey ApiKeyAuth
// @in header
// @name Authorization
// @BasePath /
// @query.collection.format multi
import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"image"
	"io"
	"log/slog"
	"net/http"
	"net/mail"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"syscall"
	"time"

	"sheff.online/aiplan/internal/aiplan/business"
	jitsi_token "sheff.online/aiplan/internal/aiplan/jitsi-token"

	"sheff.online/aiplan/internal/aiplan/cronmanager"
	"sheff.online/aiplan/internal/aiplan/types"
	"sheff.online/aiplan/internal/aiplan/utils"

	"github.com/nfnt/resize"

	"image/jpeg"
	_ "image/png"

	"github.com/gofrs/uuid"
	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo-contrib/echoprometheus"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/crypto/pbkdf2"
	"gorm.io/gorm"
	tracker "sheff.online/aiplan/internal/aiplan/activity-tracker"
	"sheff.online/aiplan/internal/aiplan/config"
	"sheff.online/aiplan/internal/aiplan/dao"
	filestorage "sheff.online/aiplan/internal/aiplan/file-storage"
	"sheff.online/aiplan/internal/aiplan/integrations"
	issues_import "sheff.online/aiplan/internal/aiplan/issues-import"
	"sheff.online/aiplan/internal/aiplan/maintenance"
	"sheff.online/aiplan/internal/aiplan/notifications"
	"sheff.online/aiplan/internal/aiplan/sessions"

	echoSwagger "github.com/swaggo/echo-swagger"
	_ "sheff.online/aiplan/internal/aiplan/docs"
)

//go:generate go run github.com/swaggo/swag/cmd/swag@latest --version
//go:generate go run github.com/swaggo/swag/cmd/swag@latest init -ot go,json --generalInfo /http.go --parseInternal --propertyStrategy snakecase --dir ./ --output docs --parseDependency 1
//go:generate echo "Generate docs"
//go:generate go run ../../cmd/docsgen/main.go -src apierrors/apierrors.go -out ../../../aiplan-help/api_errors.md
//go:generate echo "Generate schema"
//go:generate go run ../../cmd/schemagen/main.go

type Services struct {
	db                  *gorm.DB
	tracker             *tracker.ActivitiesTracker
	storage             filestorage.FileStorage
	emailService        *notifications.EmailService
	sessionsManager     *sessions.SessionsManager
	integrationsService *integrations.IntegrationsService
	importService       *issues_import.ImportService
	jitsiTokenIss       *jitsi_token.JitsiTokenIssuer

	notificationsService *notifications.Notification

	business *business.Business
}

var cfg *config.Config
var appVersion string

// ServerHeader middleware adds a `Server` header to the response.
func ServerHeader(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		c.Response().Header().Set(echo.HeaderServer, "AIPlan")
		return next(c)
	}
}

func Server(db *gorm.DB, c *config.Config, version string) {
	cfg = c
	appVersion = version

	e := echo.New()
	e.HideBanner = true
	e.HTTPErrorHandler = func(err error, c echo.Context) {
		code := http.StatusInternalServerError
		if he, ok := err.(*echo.HTTPError); ok {
			code = he.Code
		}

		// Ignore 404
		if code == http.StatusNotFound {
			c.NoContent(http.StatusNotFound)
			return
		}
		slog.Error("Unhandled error in endpoint", "url", c.Request().URL, "err", err)
		EErrorMsgStatus(c, nil, code)
	}

	storage, err := filestorage.NewMinioStorage(cfg.AWSEndpoint, cfg.AWSAccessKey, cfg.AWSSecretKey, false, cfg.AWSBucketName)
	if err != nil {
		slog.Error("Fail init Minio connection", "err", err)
		os.Exit(1)
	}

	dao.FileStorage = storage

	slog.Info("Merge users with same emails")

	slog.Info("Migrate old activities")
	activityMigrate(db) //TODO migrate to newActivities

	// Query counter
	ql := dao.NewQueryLogger()
	if err := db.Callback().
		Query().
		After("*").
		Register("instrumentation:after_query", ql.QueryCallback); err != nil {
		slog.Error("Register query callback", "err", err)
	}

	tr := tracker.NewActivitiesTracker(db)
	sm := sessions.NewSessionsManager(cfg, types.RefreshTokenExpiresPeriod+time.Hour)
	es := notifications.NewEmailService(cfg, db)
	bl := business.NewBL(db, tr)
	ns := notifications.NewNotificationService(cfg, db, tr, bl)
	np := notifications.NewNotificationProcessor(db, ns.Tg, es, ns.Ws)
	//ts := notifications.NewTelegramService(db, cfg, tracker)

	jobRegistry := cronmanager.JobRegistry{
		"notification_processing": cronmanager.Job{
			Func:     np.ProcessNotifications,
			Schedule: "* * * * *", // every minute
		},

		"email_processing": cronmanager.Job{
			Func:     es.EmailActivity,
			Schedule: fmt.Sprintf("*/%d * * * *", cfg.NotificationsSleep),
		},
		/*"delete_inactive_users": cronmanager.Job{
			Func:     maintenance.NewUserCleaner(db).DeleteInactiveUsers,
			Schedule: "0 0 * * *", // daily at midnight
		},*/
		"assets_clean": cronmanager.Job{
			Func:     maintenance.NewAssetCleaner(db, storage).CleanAssets,
			Schedule: "0 1 * * *", // daily at 01:00
		},
		"user_notification_clean": cronmanager.Job{
			Func:     notifications.NewNotificationCleaner(db).Clean,
			Schedule: "30 1 * * *", // daily at 01:30
		},
		"workspaces_clean": cronmanager.Job{
			Func:     maintenance.NewWorkspacesCleaner(db).CleanWorkspaces,
			Schedule: "0 2 * * *", // daily at 02:00
		},
		"projects_clean": cronmanager.Job{
			Func:     maintenance.NewProjectsCleaner(db).CleanProjects,
			Schedule: "0 3 * * *", // daily at 03:00
		},
	}

	// Create CronManager
	cronManager := cronmanager.NewCronManager(jobRegistry)
	if err := cronManager.LoadJobs(); err != nil {
		slog.Error("Failed to load cron jobs", "err", err)
		os.Exit(1)
	}

	s := &Services{
		db:           db,
		tracker:      tr,
		storage:      storage,
		emailService: es,
		//telegramService:       ts,
		sessionsManager: sm,
		importService:   issues_import.NewImportService(db, storage, es),
		//wsNotificationService: ws,
		notificationsService: ns,
		business:             bl,
		jitsiTokenIss:        jitsi_token.NewJitsiTokenIssuer(cfg.JitsiJWTSecret, cfg.JitsiAppID),
	}

	// Start cronManager
	cronManager.Start()

	// Create a channel to handle termination signals
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Gracefully stop NotificationProcessor
	go func() {
		<-ctx.Done()
		slog.Info("Shutting down gracefully, press Ctrl+C again to force")
		cronManager.Stop()
		es.Stop()
		os.Exit(0)
	}()

	sendPasswordDefaultAdmin(db, es)

	{ // register handler ws & UserNotify activity
		tr.RegisterHandler(notifications.NewIssueNotification(ns))
		tr.RegisterHandler(notifications.NewProjectNotification(ns))
		tr.RegisterHandler(notifications.NewDocNotification(ns))
		//tr.RegisterHandler(notifications.NewWorkspaceNotification(ns))
	}

	{ // register handler telegram activity
		tr.RegisterHandler(notifications.NewTgNotifyIssue(ns.Tg))
		tr.RegisterHandler(notifications.NewTgNotifyProject(ns.Tg))
		tr.RegisterHandler(notifications.NewTgNotifyDoc(ns.Tg))
		tr.RegisterHandler(notifications.NewTgNotifyWorkspace(ns.Tg))
	}

	// Global middlewares
	e.Use(ServerHeader)
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowCredentials: true,
	}))
	e.Use(middleware.BodyLimitWithConfig(middleware.BodyLimitConfig{
		Limit: "5M",
		Skipper: func(c echo.Context) bool {
			return c.Path() == "/api/auth/workspaces/:workspaceSlug/logo/" ||
				c.Path() == "/api/auth/workspaces/:workspaceSlug/projects/:projectId/issues/:issueIdOrSeq/issue-attachments/" ||
				c.Path() == "/api/auth/workspaces/:workspaceSlug/doc/:docId/doc-attachments/" ||
				c.Path() == "/api/auth/workspaces/:workspaceSlug/doc/" ||
				c.Path() == "/api/auth/workspaces/:workspaceSlug/doc/:docId/" ||
				c.Path() == "/api/auth/workspaces/:workspaceSlug/projects/:projectId/issues/" ||
				c.Path() == "/api/auth/workspaces/:workspaceSlug/projects/:projectId/issues/:issueIdOrSeq/" ||
				c.Path() == "/api/auth/workspaces/:workspaceSlug/projects/:projectId/issues/:issueIdOrSeq/comments/" ||
				c.Path() == "/api/auth/workspaces/:workspaceSlug/projects/:projectId/issues/:issueIdOrSeq/comments/:commentId/" ||
				c.Path() == "/api/auth/users/me/avatar/" ||
				strings.Contains(c.Path(), "/api/auth/issue-attachments/tus/") ||
				strings.Contains(c.Path(), "/api/auth/attachments/tus/")

		},
	}))
	e.Use(middleware.GzipWithConfig(middleware.GzipConfig{
		Level:     9,
		MinLength: 2048,
		Skipper: func(c echo.Context) bool {
			return c.Path() == "/api/auth/ws/notifications/" ||
				c.Path() == "/api/ws/notifications/" ||
				strings.Contains(c.Request().URL.Path, "swagger")
		},
	}))
	e.Use(echoprometheus.NewMiddleware("aiplan"))
	e.Pre(middleware.AddTrailingSlashWithConfig(middleware.TrailingSlashConfig{
		Skipper: func(c echo.Context) bool {
			return strings.Contains(c.Request().URL.Path, "swagger")
		},
	}))

	e.Validator = NewRequestValidator()

	AddAuthenticationServices(db, e, []byte(cfg.SecretKey), sm, ns.Tg, es)

	//services with auth
	apiGroup := e.Group("/api/")

	s.integrationsService = integrations.NewIntegrationService(apiGroup, db, s.notificationsService.Tg, s.storage, tr, bl)

	authGroup := apiGroup.Group("auth/",
		AuthMiddleware(AuthConfig{
			Secret:         []byte(cfg.SecretKey),
			DB:             db,
			SessionManager: sm,
		}),
	)

	apiGroup.Group("docs", middleware.StaticWithConfig(middleware.StaticConfig{
		Root:       "aiplan-help",
		Browse:     false,
		IgnoreBase: true,
		Skipper: func(c echo.Context) bool {
			ext := filepath.Ext(strings.TrimSuffix(c.Request().URL.Path, "/"))
			return !(ext == ".md" || ext == ".jpg" || ext == ".png" || ext == ".json")
		},
	}))
	apiGroup.GET("docsIndex/", NewHelpIndex("aiplan-help/"))

	authGroup.GET("queryLog/", ql.CountEndpoint)
	s.AddFormServices(authGroup) // todo
	s.AddProjectServices(authGroup)
	s.AddWorkspaceServices(authGroup)
	s.AddUserServices(authGroup)
	s.AddIssueServices(authGroup)
	s.AddBackupServices(authGroup)
	s.AddAdminServices(authGroup)
	AddProfileServices(authGroup)
	s.AddIssueMigrationServices(authGroup)
	s.AddImportServices(authGroup)
	s.AddDocServices(authGroup)

	// services without auth
	s.AddUserWithoutAuthServices(apiGroup)
	s.AddFormWithoutAuthServices(apiGroup)

	// Version endpoint
	apiGroup.GET("version/", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"version": version,
			"sign_up": cfg.SignUpEnable,
			"demo":    cfg.Demo,
			"ny":      cfg.NYEnable,
			"captcha": !cfg.CaptchaDisabled,
			"jitsi":   !cfg.JitsiDisabled,
		})
	})

	// Health endpoint
	apiGroup.GET("_health/", func(c echo.Context) error {
		return c.NoContent(http.StatusOK)
	})

	if cfg.SwaggerEnable {
		apiGroup.GET("swagger/*", echoSwagger.WrapHandler)
	}

	// Websocket notifications endpoint
	authGroup.GET("ws/notifications/", func(c echo.Context) error {
		s.notificationsService.Ws.Handle(c.(AuthContext).User.ID, c.Response(), c.Request())
		return nil
	})

	// Short urls
	e.GET("i/:slug/:projectIdent/:issueNum/", s.shortIssueURLRedirect)
	e.GET("i/:issue/", s.shortIssueURLRedirect)
	e.GET("d/:slug/:docNum/", s.shortDocURLRedirect)
	e.GET("sf/:base/", s.shortSearchFilterURLRedirect)

	// Get minio file
	apiGroup.GET("file/:fileName/", s.redirectToMinioFile)

	// Jitsi conf redirect
	authGroup.GET("conf/:room/", s.redirectToJitsiConf)

	// Front handler
	if cfg.FrontFilesPath != "" {
		slog.Info("Start front routing")
		e.Use(middleware.StaticWithConfig(middleware.StaticConfig{
			Root:  cfg.FrontFilesPath,
			HTML5: true,
			Skipper: func(c echo.Context) bool {
				return strings.Contains(c.Path(), "tus") ||
					strings.Contains(c.Path(), "swagger")
			},
		}))

		uHttp, _ := url.Parse(fmt.Sprintf("http://%s", cfg.AWSEndpoint))
		e.Group("/"+cfg.AWSBucketName,
			middleware.RemoveTrailingSlash(),
			middleware.Proxy(middleware.NewRoundRobinBalancer([]*middleware.ProxyTarget{
				{
					URL: utils.CheckHttps(uHttp),
				},
			})))
	}

	// Prometheus metrics
	go func() {
		bootTimeGauge := prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "aiplan",
			Name:      "boot_time",
			Help:      "Server startup time",
		})
		bootTimeGauge.Set(float64(time.Now().UnixMilli()))

		if err := prometheus.Register(bootTimeGauge); err != nil {
			slog.Error("Register boot time gauge", "err", err)
			os.Exit(1)
		}

		metrics := echo.New()
		metrics.HideBanner = true
		metrics.GET("/metrics", echoprometheus.NewHandler()) // adds route to serve gathered metrics
		if err := metrics.Start(":2112"); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("Metrics server fail", "err", err)
		}
	}()

	if err := e.Start(":8080"); err != nil {
		slog.Error("Server fail", "err", err)
	}
}

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
func createAccessToken(userId string) (*Token, *Token, error) {
	ta, err := GenJwtToken([]byte(cfg.SecretKey), "access", userId)
	if err != nil {
		return nil, nil, err
	}

	tr, err := GenJwtToken([]byte(cfg.SecretKey), "refresh", userId)
	if err != nil {
		return nil, nil, err
	}
	return ta, tr, err
}

func setAuthCookies(c echo.Context, accessToken *Token, refreshToken *Token) {
	accessCookie := new(http.Cookie)
	accessCookie.Name = "access_token"
	accessCookie.Value = accessToken.SignedString
	accessCookie.HttpOnly = true
	accessCookie.Secure = true
	accessCookie.Path = "/"
	accessCookie.SameSite = http.SameSiteNoneMode
	accessCookie.Expires = time.Now().Add(types.TokenExpiresPeriod)
	c.SetCookie(accessCookie)

	refreshCookie := new(http.Cookie)
	refreshCookie.Name = "refresh_token"
	refreshCookie.Value = refreshToken.SignedString
	refreshCookie.HttpOnly = true
	refreshCookie.Secure = true
	refreshCookie.Path = "/"
	refreshCookie.SameSite = http.SameSiteNoneMode
	refreshCookie.Expires = time.Now().Add(types.RefreshTokenExpiresPeriod)
	c.SetCookie(refreshCookie)
}

func clearAuthCookies(c echo.Context) {
	accessCookie := new(http.Cookie)
	accessCookie.Name = "access_token"
	accessCookie.Value = ""
	accessCookie.HttpOnly = true
	accessCookie.Secure = true
	accessCookie.Path = "/"
	accessCookie.SameSite = http.SameSiteNoneMode
	accessCookie.MaxAge = -1
	c.SetCookie(accessCookie)

	refreshCookie := new(http.Cookie)
	refreshCookie.Name = "refresh_token"
	refreshCookie.Value = ""
	refreshCookie.HttpOnly = true
	refreshCookie.Secure = true
	refreshCookie.Path = "/"
	refreshCookie.SameSite = http.SameSiteNoneMode
	refreshCookie.MaxAge = -1
	c.SetCookie(refreshCookie)
}

type Token struct {
	JWT          *jwt.Token
	SignedString string
	Type         string
}

// Генерация JWT ключа
func GenJwtToken(secret []byte, tokenType string, userid string) (*Token, error) {
	u, _ := uuid.NewV4()
	claims := jwt.MapClaims{
		"exp":        jwt.NewNumericDate(time.Now().Add(types.TokenExpiresPeriod)),
		"iat":        jwt.NewNumericDate(time.Now()),
		"jti":        fmt.Sprintf("%x", u),
		"token_type": tokenType,
		"user_id":    userid,
	}
	if tokenType == "refresh" {
		claims["exp"] = jwt.NewNumericDate(time.Now().Add(types.RefreshTokenExpiresPeriod))
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedString, err := token.SignedString(secret)
	if err != nil {
		return nil, err
	}

	// Waiting for PR https://github.com/golang-jwt/jwt/pull/417
	sigStr := signedString[strings.LastIndex(signedString, ".")+1:]
	sig, err := base64.RawURLEncoding.DecodeString(sigStr)
	if err != nil {
		return nil, err
	}
	token.Signature = sig

	return &Token{
		JWT:          token,
		SignedString: signedString,
		Type:         tokenType,
	}, nil
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

func rapidoc(c echo.Context) error {
	type PageData struct {
		SwaggerURL string
	}
	data := PageData{
		SwaggerURL: fmt.Sprint(cfg.WebURL) + "/api/swagger/docs/",
	}

	tmpl, err := template.ParseFiles("docs/index.html")
	if err != nil {
		return c.String(http.StatusInternalServerError, "Template parsing error")
	}

	return tmpl.Execute(c.Response().Writer, data)
}

func GetActivitiesTable(query *gorm.DB, from DayRequest, to DayRequest) (map[string]types.ActivityTable, error) {
	var activities []struct {
		ActorId string
		Day     time.Time
		Cnt     int
	}
	if err := query.
		Select("actor_id, date_trunc('day', created_at) as Day, count(*) as Cnt").
		Where("created_at between ? and ?", time.Time(from), time.Time(to)).
		Where("actor_id is not null").
		Group("actor_id, Day").
		Order("Day").
		Model(&dao.FullActivity{}).
		Find(&activities).Error; err != nil {
		return nil, err
	}

	resp := make(map[string]types.ActivityTable)
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

func sendPasswordDefaultAdmin(tx *gorm.DB, es *notifications.EmailService) {
	var user dao.User
	if err := tx.Where("username = ?", "admin").Where("is_onboarded = ?", false).First(&user).Error; err != nil {
		return
	}

	err := es.NewUserPasswordNotify(user, "password123")
	if err != nil {
		return
	}
}
