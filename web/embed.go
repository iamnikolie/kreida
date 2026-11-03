// Package web carries the page: the Vite build of web/src, embedded so the binary serves it alone.
package web

import "embed"

// Dist is the output of `pnpm build` in web/. A checkout that never built the page holds only dist/.keep, which the
// server answers with a "page not built" message instead of failing to compile.
//
//go:embed all:dist
var Dist embed.FS
