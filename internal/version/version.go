package version

// Version is the build version string.
//
// It is typically set at build time via:
//
//	-ldflags "-X manifold/internal/version.Version=<version>"
//
// The default is "dev".
var Version = "dev"
