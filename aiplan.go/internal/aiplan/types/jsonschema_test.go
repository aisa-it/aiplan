package types

import (
	"fmt"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestValidateOptions(t *testing.T) {
	schema := GenValueSchema("select", []string{"2222", "2211"})
	fmt.Printf("Generated schema: %+v\n", schema)
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("schema.json", schema); err != nil {
		t.Fatal(err)
	}

	sch, err := compiler.Compile("schema.json")
	if err != nil {
		t.Fatal(err)
	}

	fmt.Println(sch.Validate(nil))
}
