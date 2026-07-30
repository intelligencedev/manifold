// Package assets embeds files that ship with the Manifold binary, such as the
// bundled skills copied into the Manifold home directory on startup.
package assets

import "embed"

// Skills contains the bundled skills under assets/skills. Each immediate
// subdirectory is one skill (a directory with a SKILL.md plus optional
// references/scripts). These are installed into <manifoldHome>/skills at boot.
//
//go:embed skills
var Skills embed.FS
