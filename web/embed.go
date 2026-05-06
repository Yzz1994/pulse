package web

import "embed"

// PanelDistFS embeds the built React SPA from web/panel/dist/.
// Build the SPA first: cd web/panel && bun install && bun run build.ts
//
//go:embed all:panel/dist
var PanelDistFS embed.FS
