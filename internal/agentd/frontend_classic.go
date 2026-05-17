//go:build !cockpit

package agentd

import "manifold/internal/webui"

type frontendOptions = webui.Options

var registerFrontendUI = webui.RegisterFrontend
