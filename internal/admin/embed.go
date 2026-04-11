package admin

import "embed"

// UIAssets contains the compiled Next.js static export.
// Build with: cd web && npm run build
// Then copy: cp -r web/out internal/admin/out
//
//go:embed all:out
var UIAssets embed.FS
