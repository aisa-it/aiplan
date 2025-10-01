package jitsi_token

import (
	"time"

	"github.com/aisa-it/aiplan/internal/aiplan/dao"
	"github.com/golang-jwt/jwt/v4"
)

type JitsiTokenIssuer struct {
	signKey string
	appID   string
}

func NewJitsiTokenIssuer(signKey, appID string) *JitsiTokenIssuer {
	return &JitsiTokenIssuer{signKey: signKey, appID: appID}
}

func (jti *JitsiTokenIssuer) IssueToken(user *dao.User, isModerator bool, room string) (string, error) {
	claims := jwt.MapClaims{
		"exp": jwt.NewNumericDate(time.Now().Add(time.Minute)),
		"iss": jti.appID,
		"aud": "jitsi",
		"context": map[string]any{
			"user": map[string]any{
				"avatar":    user.Avatar,
				"name":      user.GetName(),
				"email":     user.Email,
				"id":        user.ID,
				"moderator": isModerator,
			},
		},
	}

	if room != "" {
		claims["room"] = room
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	ret, err := token.SignedString([]byte(jti.signKey))
	if err != nil {
		return "", err
	}
	return ret, err
}
