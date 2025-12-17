//go:build embedSPA
// +build embedSPA

package aiplan

import "embed"

//go:embed spa
var frontFS embed.FS
