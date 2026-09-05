// Package schemas carries the knowledge JSON Schema documents as embedded
// assets (tech-stack §111), so a released binary can validate knowledge records
// without reaching the network (tech-stack §39).
//
// The package exists only because go:embed cannot reach above its own
// directory: tech-stack §7 puts the documents at the repository root under
// schemas/knowledge/, so the embed.FS declaration has to live beside them
// rather than in internal/knowledge/schema (decision D-33).
package schemas

import "embed"

// KnowledgeFS holds every knowledge schema document, rooted at the repository
// so that paths inside it read "knowledge/<name>.json".
//
//go:embed knowledge/*.json
var KnowledgeFS embed.FS
