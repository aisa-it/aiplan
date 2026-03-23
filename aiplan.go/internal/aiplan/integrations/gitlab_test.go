package integrations

import (
	"fmt"
	"testing"
)

func TestGitlab(t *testing.T) {
	fmt.Println(ParseMessage("EDOSF-2694 EDOSF-2695 5 17.5 13 23 27 30 31 32 33\n"))
}
