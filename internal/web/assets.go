// ABOUTME: Embeds the single-page browser UI served by the web command.
// ABOUTME: Kept as one self-contained file so the binary stays dependency-free.
package web

import _ "embed"

//go:embed index.html
var indexHTML string
