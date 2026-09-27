// Package ai embeds ai/skills -- cover100's canonical Agent Skills --
// directly into the cover100 binary.
package ai

import "embed"

// SkillsFS holds every file under skills/ at build time: skills/<name>/
// SKILL.md plus each skill's references/ subdirectories and documentation.
//
//go:embed all:skills
var SkillsFS embed.FS
