// Пакет для аутентификации и авторизации пользователей в приложении AiPlan.
// Обеспечивает безопасный доступ к ресурсам, используя JWT, куки и капчу.
//
// Основные возможности:
//   - Аутентификация пользователей по email и паролю с использованием капчи.
//   - Генерация и проверка токенов доступа (JWT) с поддержкой обновления.
//   - Защита от повторных атак и блокировка аккаунтов при неудачных попытках входа.
//   - Интеграция с Sesion Manager для управления сессиями пользователей.
//   - Поддержка различных схем аутентификации (Basic, Bearer, Cookies).
package aiplan

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/apierrors"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/notifications"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/sessions"
	"github.com/altcha-org/altcha-lib-go"
	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"gorm.io/gorm"
)

type Authentication struct {
	db              *gorm.DB
	secret          []byte
	sessionsManager *sessions.SessionsManager
	telegramService *notifications.TelegramService
	emailService    *notifications.EmailService
}

type AuthContext struct {
	echo.Context
	User         *dao.User
	AccessToken  *Token
	RefreshToken *Token
	TokenAuth    bool
}

type AuthConfig struct {
	Secret         []byte
	DB             *gorm.DB
	SessionManager *sessions.SessionsManager
	Skipper        middleware.Skipper
}

func AuthMiddleware(config AuthConfig) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if c.Request().Method == "OPTIONS" {
				return c.NoContent(http.StatusOK)
			}

			if config.Skipper != nil && config.Skipper(c) {
				return next(c)
			}

			var refreshToken *Token
			var accessToken *Token

			schema, tokenString, ok := strings.Cut(c.Request().Header.Get("Sec-WebSocket-Protocol"), ",")
			if !ok {
				schema, tokenString, ok = strings.Cut(c.Request().Header.Get("Authorization"), " ")
				if !ok {
					// Cookie token
					schema = "Cookies"
					if accessCookie, err := c.Cookie("access_token"); err == nil || accessCookie != nil {
						accessToken = new(Token)
						accessToken.SignedString = accessCookie.Value
						accessToken.Type = "access"
					}

					if refreshCookie, err := c.Cookie("refresh_token"); err == nil || refreshCookie != nil {
						refreshToken = new(Token)
						refreshToken.SignedString = refreshCookie.Value
						refreshToken.Type = "refresh"
					}

					if refreshToken == nil && accessToken == nil {
						return EErrorDefined(c, apierrors.ErrTokenInvalid)
					}
				}
			}
			schema = strings.TrimSpace(schema)

			if schema != "Cookies" {
				accessToken = new(Token)
				accessToken.SignedString = strings.TrimSpace(tokenString)
				accessToken.Type = schema
			}

			// Token auth
			if schema == "Basic" || schema == "Bearer" {
				var user dao.User
				if err := config.DB.
					Joins("LastWorkspace").
					Joins("Tariffication").
					Where("users.auth_token = ?", accessToken.SignedString).
					First(&user).Error; err != nil {
					if err == gorm.ErrRecordNotFound {
						return EErrorDefined(c, apierrors.ErrFailedLogin)
					}
				}
				if err := dao.UpdateUserLastActivityTime(config.DB, &user); err != nil {
					EError(c, err)
				}
				return next(AuthContext{c, &user, accessToken, nil, true})
			}

			var err error
			keyFunc := func(token *jwt.Token) (interface{}, error) {
				if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
				}
				return config.Secret, nil
			}

			var accessError error
			if accessToken != nil {
				accessToken.JWT, accessError = jwt.Parse(accessToken.SignedString, keyFunc)
			}

			var refreshError error
			if refreshToken != nil {
				refreshToken.JWT, refreshError = jwt.Parse(refreshToken.SignedString, keyFunc)
				if refreshError != nil {
					return EErrorDefined(c, apierrors.ErrTokenInvalid)
				}
			}

			var user *dao.User

			// Prolong if expired
			if errors.Is(accessError, jwt.ErrTokenExpired) || accessToken == nil {
				accessToken, user, err = config.tokenProlong(c, refreshToken)
				if accessToken == nil || user == nil {
					return err
				}
			} else if accessError != nil {
				if accessToken.JWT != nil && !accessToken.JWT.Valid {
					return EErrorDefined(c, apierrors.ErrTokenInvalid)
				}
				return EError(c, err)
			} else {
				// Check if token not blacklisted
				blacklisted, err := config.SessionManager.IsTokenBlacklisted(accessToken.JWT.Signature)
				if err != nil {
					return EError(c, err)
				}

				if blacklisted {
					return EErrorDefined(c, apierrors.ErrTokenExpired)
				}

				claims, ok := accessToken.JWT.Claims.(jwt.MapClaims)
				if !ok || !accessToken.JWT.Valid {
					return EErrorDefined(c, apierrors.ErrTokenInvalid)
				}
				user = new(dao.User)
				user.ID = claims["user_id"].(string)

				// Fetch user
				if err := config.DB.
					Joins("LastWorkspace").
					Joins("AvatarAsset").
					Joins("Tariffication").
					First(user).Error; err != nil {
					return EErrorDefined(c, apierrors.ErrTokenInvalid)
				}

				//Check if token older than session reseted
				issued, ok := claims["iat"].(float64)
				if !ok {
					return EErrorDefined(c, apierrors.ErrTokenInvalid)
				}

				var reseted sql.NullBool
				if err := config.DB.Model(&dao.SessionsReset{}).
					Select("? < max(reseted_at)", time.Unix(int64(issued), 0)).
					Where("user_id = ?", user.ID).Find(&reseted).Error; err != nil {
					return EError(c, err)
				}

				if reseted.Valid && reseted.Bool {
					return EErrorDefined(c, apierrors.ErrSessionReset)
				}
			}

			if user == nil {
				return EError(c, errors.New("nil user"))
			}

			// If user blocked
			if !user.IsActive {
				tm := time.Now()
				user.LastLogoutTime = &tm
				user.LastLogoutIp = c.RealIP()
				if err := config.DB.Model(user).Select("LastLogoutTime", "LastLogoutIp").Updates(user).Error; err != nil {
					return EError(c, err)
				}

				//Reset all user sessions
				if err := dao.ResetUserSessions(config.DB, user); err != nil {
					return EError(c, err)
				}

				return EErrorDefined(c, apierrors.ErrSessionReset)
			}

			if err := dao.UpdateUserLastActivityTime(config.DB, user); err != nil {
				EError(c, err)
			}

			return next(AuthContext{c, user, accessToken, refreshToken, false})
		}
	}
}

func AddAuthenticationServices(db *gorm.DB, g *echo.Echo, secret []byte, sessionManager *sessions.SessionsManager, telegramService *notifications.TelegramService, emailService *notifications.EmailService) *Authentication {
	ret := &Authentication{db, secret, sessionManager, telegramService, emailService}

	g.POST("api/sign-in/", ret.emailLogin)

	g.GET("api/captcha/", ret.requestCaptcha)
	return ret
}

type LoginRequest struct {
	Email          string `json:"email"`
	Password       string `json:"password"`
	CaptchaPayload string `json:"captcha_payload"`
}

// emailLogin godoc
// @id emailLogin
// @Summary Пользователи (управление доступом): вход пользователя
// @Description Аутентифицирует пользователя с использованием email и пароля, с проверкой капчи
// @Tags Users
// @Accept json
// @Produce json
// @Param data body LoginRequest true "Данные для входа пользователя"
// @Success 200 {object} map[string]interface{} "Токены доступа и информация о пользователе"
// @Failure 400 {object} apierrors.DefinedError "Некорректные данные запроса или неудачная проверка капчи"
// @Failure 401 {object} apierrors.DefinedError "Неудачный вход в систему"
// @Failure 500 {object} apierrors.DefinedError "Внутренняя ошибка сервера"
// @Router /api/sign-in [post]
func (a *Authentication) emailLogin(c echo.Context) error {
	var req LoginRequest
	if err := c.Bind(&req); err != nil {
		return EError(c, err)
	}

	req.Email = strings.ToLower(req.Email)

	if !CaptchaService.Validate(req.CaptchaPayload) {
		return EErrorDefined(c, apierrors.ErrCaptchaFail)
	}

	if req.Email == "" || req.Password == "" {
		return EErrorDefined(c, apierrors.ErrLoginCredentialsRequired)
	}

	if !ValidateEmail(req.Email) {
		return EErrorDefined(c, apierrors.ErrInvalidEmail)
	}

	var user dao.User
	if err := a.db.Where("email = ?", req.Email).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return EErrorDefined(c, apierrors.ErrFailedLogin)
		}
		return EError(c, err)
	}

	if user.BlockedUntil.Valid && user.BlockedUntil.Time.After(time.Now()) {
		return EErrorDefined(c, apierrors.ErrBlockedUntil.WithFormattedMessage(user.BlockedUntil.Time.Format("02.01.2006 15:04")))
	}

	if !user.IsActive {
		return EErrorDefined(c, apierrors.ErrLoginTriesExceed)
	}

	if user.IsIntegration {
		return EErrorDefined(c, apierrors.ErrIntegrationLogin)
	}

	if !checkPassword(req.Password, user.Password) {
		user.LoginAttempts++
		time.Sleep(time.Second * time.Duration(user.LoginAttempts))

		// block after 5 fails
		if user.LoginAttempts >= 5 {
			slog.Info("Block user for more than 5 failed attempts", "user", user.String())
			user.BlockedUntil = sql.NullTime{Valid: true, Time: time.Now().Add(time.Minute * 20)}
			user.LoginAttempts = 0
		}

		if err := a.db.Model(&user).Select("LoginAttempts", "BlockedUntil").Updates(&user).Error; err != nil {
			return EError(c, err)
		}

		if user.BlockedUntil.Valid && user.BlockedUntil.Time.After(time.Now()) {
			a.telegramService.UserBlockedUntil(user, user.BlockedUntil.Time)
			a.emailService.UserBlockedUntil(user, user.BlockedUntil.Time)
			return EErrorDefined(c, apierrors.ErrBlockedUntil.WithFormattedMessage(user.BlockedUntil.Time.Format("02.01.2006 15:04")))
		}

		return EErrorDefined(c, apierrors.ErrFailedLogin)
	}

	tm := time.Now()

	user.LastActive = &tm
	user.LastLoginTime = &tm

	user.LastLoginIp = c.RealIP()
	user.LastLoginUagent = c.Request().UserAgent()
	user.TokenUpdatedAt = &tm
	user.LoginAttempts = 0
	user.BlockedUntil = sql.NullTime{Valid: false}
	if err := a.db.Model(&user).Select("LastActive", "LastLoginTime", "LastLoginIp", "LastLoginUagent", "TokenUpdatedAt", "LoginAttempts", "BlockedUntil").Updates(&user).Error; err != nil {
		return EError(c, err)
	}

	access_token, refresh_token, err := createAccessToken(user.ID)
	if err != nil {
		return EError(c, err)
	}

	setAuthCookies(c, access_token, refresh_token)

	return c.JSON(http.StatusOK, map[string]interface{}{
		"access_token":  access_token.SignedString,
		"refresh_token": refresh_token.SignedString,
		"user":          user,
	})
}

func (a *AuthConfig) tokenProlong(c echo.Context, token *Token) (*Token, *dao.User, error) {
	if token == nil {
		return nil, nil, EErrorDefined(c, apierrors.ErrRefreshTokenRequired)
	}
	// Check if token not blacklisted
	{
		blacklisted, err := a.SessionManager.IsTokenBlacklisted(token.JWT.Signature)
		if err != nil {
			EError(c, err)
			return nil, nil, EErrorDefined(c, apierrors.ErrTokenExpired)
		}

		if blacklisted {
			return nil, nil, EErrorDefined(c, apierrors.ErrTokenExpired)
		}
	}

	// Blacklist old refresh token
	if err := a.SessionManager.BlacklistToken(token.JWT.Signature); err != nil {
		return nil, nil, EError(c, err)
	}

	claims, ok := token.JWT.Claims.(jwt.MapClaims)
	if !ok || !token.JWT.Valid {
		return nil, nil, EErrorDefined(c, apierrors.ErrTokenInvalid)
	}

	var user dao.User
	if err := a.DB.
		Joins("LastWorkspace").
		Joins("AvatarAsset").
		Joins("Tariffication").
		Where("users.id = ?", claims["user_id"].(string)).
		First(&user).Error; err != nil {
		return nil, nil, EErrorDefined(c, apierrors.ErrTokenInvalid)
	}

	//Check if token older than session reseted
	issued, ok := claims["iat"].(float64)
	if !ok {
		return nil, nil, EErrorDefined(c, apierrors.ErrTokenInvalid)
	}

	var reseted sql.NullBool
	if err := a.DB.Model(&dao.SessionsReset{}).
		Select("? < max(reseted_at)", time.Unix(int64(issued), 0)).
		Where("user_id = ?", user.ID).Find(&reseted).Error; err != nil {
		return nil, nil, EError(c, err)
	}

	if reseted.Valid && reseted.Bool {
		return nil, nil, EErrorDefined(c, apierrors.ErrSessionReset)
	}

	accessToken, refreshToken, err := createAccessToken(user.ID)
	if err != nil {
		return nil, nil, EError(c, err)
	}

	setAuthCookies(c, accessToken, refreshToken)

	return accessToken, &user, nil
}

// requestCaptcha godoc
// @id requestCaptcha
// @Summary Пользователи (управление доступом): запрос капчи для пользователя
// @Description Генерирует и возвращает вызов капчи для пользователя
// @Tags Users
// @Accept json
// @Produce json
// @Success 200 {object} altcha.Challenge "Капча успешно создана"
// @Failure 500 {object} apierrors.DefinedError "Внутренняя ошибка сервера"
// @Router /api/captcha [get]
func (a *Authentication) requestCaptcha(c echo.Context) error {
	expires := time.Now().Add(AltchaExpires)
	challenge, err := altcha.CreateChallenge(altcha.ChallengeOptions{
		HMACKey:   AltchaHMACKey,
		MaxNumber: 10000,
		Expires:   &expires,
		Params:    url.Values{},
	})
	if err != nil {
		return EError(c, err)
	}
	return c.JSON(http.StatusOK, challenge)
}

func getUserIdFromJWT(token string) (string, error) {
	d, err := base64.StdEncoding.DecodeString(strings.Split(token, ".")[1])
	if err != nil {
		return "", err
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(d, &payload); err != nil {
		return "", err
	}
	return payload["user_id"].(string), nil
}
