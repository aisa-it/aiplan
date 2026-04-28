package token

import (
	"fmt"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	"github.com/gofrs/uuid"
	"github.com/golang-jwt/jwt/v5"
)

type Token struct {
	JWT          *jwt.Token
	SignedString string
	Type         string
}

// Генерация JWT ключа
func GenJwtToken(secret []byte, tokenType string, userid uuid.UUID) (*Token, error) {
	u, _ := uuid.NewV4()
	claims := jwt.MapClaims{
		"exp":        jwt.NewNumericDate(time.Now().Add(types.TokenExpiresPeriod)),
		"iat":        jwt.NewNumericDate(time.Now()),
		"jti":        fmt.Sprintf("%x", u),
		"token_type": tokenType,
		"user_id":    userid.String(),
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
	/*sigStr := signedString[strings.LastIndex(signedString, ".")+1:]
	sig, err := base64.RawURLEncoding.DecodeString(sigStr)
	if err != nil {
		return nil, err
	}
	token.Signature = sig*/

	return &Token{
		JWT:          token,
		SignedString: signedString,
		Type:         tokenType,
	}, nil
}
