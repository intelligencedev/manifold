//go:build cockpit

package agentd

import "manifold/internal/cockpitui"

type frontendOptions = cockpitui.Options

var registerFrontendUI = cockpitui.RegisterFrontend
