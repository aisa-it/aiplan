package jitsi_token

import (
	"fmt"
	"testing"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
)

func TestIssueToken(t *testing.T) {
	isr := NewJitsiTokenIssuer("", "test_aiplan")
	token, err := isr.IssueToken(&dao.User{
		FirstName: "Egor",
		LastName:  "Shevtsov",
		Email:     "egor@aisa.ru",
		ID:        dao.GenID(),
	}, true, "aiplan")
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("https://test-jitsi.aisa.ru/aiplan?jwt=" + token)
}
