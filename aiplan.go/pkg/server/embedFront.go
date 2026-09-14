//go:build embedSPA
// +build embedSPA

package server

import "embed"

//go:embed spa
var frontFS embed.FS
