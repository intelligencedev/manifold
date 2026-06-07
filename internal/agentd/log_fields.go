package agentd

const (
	logFieldWorkspaceRefSnake = "workspace_ref"
	logFieldWorkspaceRefCamel = "workspaceRef"
)

func workspaceRefFieldSnake() string {
	return logFieldWorkspaceRefSnake
}

func workspaceRefFieldCamel() string {
	return logFieldWorkspaceRefCamel
}
