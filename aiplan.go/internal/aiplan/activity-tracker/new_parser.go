package tracker

import (
	"strings"
)

func ParseActivityTag(tag string) (ActivityFieldSpec, error) {
	spec := ActivityFieldSpec{Kind: "scalar", PreserveID: false} // default - не сохранять ID

	for _, param := range strings.Split(tag, ";") {
		parts := strings.SplitN(param, ":", 2)
		if len(parts) != 2 {
			continue
		}

		switch parts[0] {
		case "req":
			spec.Req = parts[1]
		case "field":
			spec.Field = parts[1]
		case "kind":
			spec.Kind = parts[1]
		case "transform":
			spec.Transform = parts[1]
		case "table":
			spec.Table = parts[1]
		case "preserve_id":
			spec.PreserveID = parts[1] == "true"
		case "linked_field":
			spec.LinkedField = parts[1]
		}
	}

	return spec, nil
}
