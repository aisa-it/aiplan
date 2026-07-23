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

	apicontext "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/api-context"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/apierrors"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/token"
	tokenscache "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/tokens-cache"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"

	mem "github.com/aisa-it/aiplan-mem/api"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/altcha-org/altcha-lib-go"
	"github.com/gofrs/uuid"
	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"gorm.io/gorm"
)

type AuthConfig struct {
	Secret      []byte
	DB          *gorm.DB
	MemDB       *mem.AIPlanMemAPI
	Skipper     middleware.Skipper
	TokensCache *tokenscache.TokensCache
}

func (s *Services) AuthMiddleware(secret []byte, skipper middleware.Skipper) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx, span := otel.
				Tracer("aiplan").
				Start(c.Request().Context(), "auth")
			defer span.End()

			if c.Request().Method == "OPTIONS" {
				return c.NoContent(http.StatusOK)
			}

			if strings.Contains(c.Path(), "/tus/") &&
				(c.Request().Method == http.MethodPatch ||
					c.Request().Method == http.MethodGet ||
					c.Request().Method == http.MethodDelete) {
				return next(c)
			}

			if skipper != nil && skipper(c) {
				return next(c)
			}

			var refreshToken *token.Token
			var accessToken *token.Token

			schema, tokenString, ok := strings.Cut(c.Request().Header.Get("Sec-WebSocket-Protocol"), ",")
			if !ok {
				schema, tokenString, ok = strings.Cut(c.Request().Header.Get("Authorization"), " ")
				if !ok {
					// Cookie token
					schema = "Cookies"
					if accessCookie, err := c.Cookie("access_token"); err == nil || accessCookie != nil {
						accessToken = new(token.Token)
						accessToken.SignedString = accessCookie.Value
						accessToken.Type = "access"
					}

					if refreshCookie, err := c.Cookie("refresh_token"); err == nil || refreshCookie != nil {
						refreshToken = new(token.Token)
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
				accessToken = new(token.Token)
				accessToken.SignedString = strings.TrimSpace(tokenString)
				accessToken.Type = schema
			}

			// Token auth
			if schema == "Basic" || schema == "Bearer" {
				var user dao.User
				if err := s.db.WithContext(ctx).
					Joins("LastWorkspace").
					Where("users.auth_token = ?", accessToken.SignedString).
					First(&user).Error; err != nil {
					if err == gorm.ErrRecordNotFound {
						return EErrorDefined(c, apierrors.ErrFailedLogin)
					}
					span.RecordError(err)
					span.SetStatus(codes.Error, "user lookup failed")
					return EError(c, err)
				}
				if err := dao.UpdateUserLastActivityTime(s.DB(c), &user); err != nil {
					span.RecordError(err)
					span.SetStatus(codes.Error, "user last activity update failed")
					EError(c, err)
				}
				c.Set("user", &user)
				apicontext.SetContext(c, s.db.WithContext(ctx), &apicontext.UserMeta{
					User:         &user,
					AccessToken:  accessToken,
					RefreshToken: nil,
					TokenAuth:    true,
				})
				return next(c)
			}

			var err error
			keyFunc := func(token *jwt.Token) (interface{}, error) {
				if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
				}
				return secret, nil
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
				accessToken, user, err = s.tokenProlong(c, refreshToken)
				if accessToken == nil || user == nil {
					return err
				}
			} else if accessError != nil {
				if accessToken.JWT != nil && !accessToken.JWT.Valid {
					return EErrorDefined(c, apierrors.ErrTokenInvalid)
				}
				span.RecordError(err)
				span.SetStatus(codes.Error, "access token issue failed")
				return EError(c, err)
			} else {
				// Check if token not blacklisted
				blacklisted, err := s.memDB.IsTokenBlacklisted(accessToken.JWT.Signature)
				if err != nil {
					span.RecordError(err)
					span.SetStatus(codes.Error, "token blacklist failed")
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
				user.ID = uuid.Must(uuid.FromString(claims["user_id"].(string)))

				// Fetch user
				if err := s.db.WithContext(ctx).
					Joins("LastWorkspace").
					Joins("AvatarAsset").
					First(user).Error; err != nil {
					return EErrorDefined(c, apierrors.ErrTokenInvalid)
				}

				//Check if token older than session reseted
				issued, ok := claims["iat"].(float64)
				if !ok {
					return EErrorDefined(c, apierrors.ErrTokenInvalid)
				}

				var reseted sql.NullBool
				if err := s.db.WithContext(ctx).Model(&dao.SessionsReset{}).
					Select("? < max(reseted_at)", time.Unix(int64(issued), 0)).
					Where("user_id = ?", user.ID).Find(&reseted).Error; err != nil {
					span.RecordError(err)
					span.SetStatus(codes.Error, "get session reset status failed")
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
				if err := s.db.WithContext(ctx).Model(user).Select("LastLogoutTime", "LastLogoutIp").Updates(user).Error; err != nil {
					span.RecordError(err)
					span.SetStatus(codes.Error, "update user last login info failed")
					return EError(c, err)
				}

				//Reset all user sessions
				if err := dao.ResetUserSessions(s.db.WithContext(ctx), user); err != nil {
					span.RecordError(err)
					span.SetStatus(codes.Error, "reset user sessions failed")
					return EError(c, err)
				}

				return EErrorDefined(c, apierrors.ErrSessionReset)
			}

			if err := dao.UpdateUserLastActivityTime(s.db.WithContext(ctx), user); err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, "update user last activity failed")
				EError(c, err)
			}

			user.Email = strings.ToLower(user.Email)

			c.Set("user", user)
			apicontext.SetContext(c, s.db.WithContext(ctx), &apicontext.UserMeta{
				User:         user,
				AccessToken:  accessToken,
				RefreshToken: refreshToken,
				TokenAuth:    false,
			})

			return next(c)
		}
	}
}

func (s *Services) AddAuthenticationServices(g *echo.Group, secret []byte) {
	g.POST("sign-in/", s.emailLogin)

	g.GET("captcha/", s.requestCaptcha)
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
func (s *Services) emailLogin(c echo.Context) error {
	var req LoginRequest
	if err := c.Bind(&req); err != nil {
		return EError(c, err)
	}

	req.Email = strings.ToLower(req.Email)

	// Validation
	{
		if !cfg.CaptchaDisabled && !CaptchaService.Validate(req.CaptchaPayload) {
			return EErrorDefined(c, apierrors.ErrCaptchaFail)
		}

		if req.Email == "" || req.Password == "" {
			return EErrorDefined(c, apierrors.ErrLoginCredentialsRequired)
		}

		if !ValidateEmail(req.Email) {
			return EErrorDefined(c, apierrors.ErrInvalidEmail)
		}
	}

	var user dao.User
	if err := s.DB(c).Where("email = ?", req.Email).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			if s.authProvider != nil && s.authProvider.AuthUser(req.Email, req.Password) {
				slog.InfoContext(c.Request().Context(), "Create new user from LDAP", "email", req.Email)

				user = dao.User{
					ID:           dao.GenUUID(),
					Email:        req.Email,
					Password:     dao.GenPasswordHash(req.Password),
					Theme:        types.DefaultTheme,
					IsActive:     true,
					IsOnboarded:  false,
					AuthProvider: "ldap",
				}

				if err := s.DB(c).Create(&user).Error; err != nil {
					return EError(c, err)
				}
			} else {
				return EErrorDefined(c, apierrors.ErrFailedLogin)
			}
		} else {
			return EError(c, err)
		}
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

	sucessfullLogin := false
	if s.authProvider != nil {
		if s.authProvider.AuthUser(req.Email, req.Password) {
			// If LDAP auth success - update user password to LDAP password
			if user.AuthProvider != "ldap" {
				if err := s.DB(c).Model(&user).Updates(map[string]any{
					"password":      dao.GenPasswordHash(req.Password),
					"auth_provider": "ldap",
				}).Error; err != nil {
					return EError(c, err)
				}
			}
			sucessfullLogin = true
		}
	}

	if !cfg.LDAPForce && checkPassword(req.Password, user.Password) {
		sucessfullLogin = true
	}

	if !sucessfullLogin {
		user.LoginAttempts++
		time.Sleep(time.Second * time.Duration(user.LoginAttempts))

		// block after 5 fails
		if user.LoginAttempts >= 5 {
			slog.InfoContext(c.Request().Context(), "Block user for more than 5 failed attempts", "user", user.String())
			user.BlockedUntil = sql.NullTime{Valid: true, Time: time.Now().Add(time.Minute * 20)}
			user.LoginAttempts = 0
		}

		if err := s.DB(c).Model(&user).Select("LoginAttempts", "BlockedUntil").Updates(&user).Error; err != nil {
			return EError(c, err)
		}

		if user.BlockedUntil.Valid && user.BlockedUntil.Time.After(time.Now()) {
			s.notificationsService.Tg.UserBlockedUntil(user, user.BlockedUntil.Time)
			s.emailService.UserBlockedUntil(user, user.BlockedUntil.Time)
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
	user.BlockedUntil = sql.NullTime{}
	if err := s.DB(c).Model(&user).Select("LastActive", "LastLoginTime", "LastLoginIp", "LastLoginUagent", "TokenUpdatedAt", "LoginAttempts", "BlockedUntil").Updates(&user).Error; err != nil {
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

func (s *Services) tokenProlong(c echo.Context, tkn *token.Token) (*token.Token, *dao.User, error) {
	if tkn == nil {
		return nil, nil, EErrorDefined(c, apierrors.ErrRefreshTokenRequired)
	}

	if cachedTokens := s.tokensCache.GetTokens(tkn.SignedString); cachedTokens != nil {
		accessToken := &token.Token{SignedString: cachedTokens.AccessToken}
		refreshToken := &token.Token{SignedString: cachedTokens.RefreshToken}
		setAuthCookies(c, accessToken, refreshToken)
		return accessToken, cachedTokens.User, nil
	}

	// Check if token not blacklisted
	{
		blacklisted, err := s.memDB.IsTokenBlacklisted(tkn.JWT.Signature)
		if err != nil {
			EError(c, err)
			return nil, nil, EErrorDefined(c, apierrors.ErrTokenExpired)
		}

		if blacklisted {
			return nil, nil, EErrorDefined(c, apierrors.ErrTokenExpired)
		}
	}

	// Blacklist old refresh token
	if err := s.memDB.BlacklistToken(tkn.JWT.Signature); err != nil {
		return nil, nil, EError(c, err)
	}

	claims, ok := tkn.JWT.Claims.(jwt.MapClaims)
	if !ok || !tkn.JWT.Valid {
		return nil, nil, EErrorDefined(c, apierrors.ErrTokenInvalid)
	}

	var user dao.User
	if err := s.DB(c).
		Joins("LastWorkspace").
		Joins("AvatarAsset").
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
	if err := s.DB(c).Model(&dao.SessionsReset{}).
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

	s.tokensCache.StoreTokens(tkn.SignedString, accessToken.SignedString, refreshToken.SignedString, &user)

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
func (s *Services) requestCaptcha(c echo.Context) error {
	expires := time.Now().Add(AltchaExpires)
	challenge, err := altcha.CreateChallenge(altcha.ChallengeOptions{
		HMACKey:   cfg.CaptchaSecret,
		MaxNumber: 10000,
		Expires:   &expires,
		Params:    url.Values{},
	})
	if err != nil {
		return EError(c, err)
	}
	return c.JSON(http.StatusOK, challenge)
}

func getUserIdFromJWT(token string) (uuid.UUID, error) {
	d, err := base64.StdEncoding.DecodeString(strings.Split(token, ".")[1])
	if err != nil {
		return uuid.Nil, err
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(d, &payload); err != nil {
		return uuid.Nil, err
	}
	return uuid.FromString(payload["user_id"].(string))
}
