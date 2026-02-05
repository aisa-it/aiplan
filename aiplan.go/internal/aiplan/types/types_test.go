package types

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestUUID(t *testing.T) {
	s := `["5E838708-6D06-4187-AB8D-E6A9073B33F3", ""]`
	var ss FilterUUIDs
	fmt.Println(json.Unmarshal([]byte(s), &ss))
	fmt.Println(ss.Array, ss.IncludeEmpty)

	d, err := json.Marshal(ss)
	fmt.Println(err)
	fmt.Println(string(d))
}
