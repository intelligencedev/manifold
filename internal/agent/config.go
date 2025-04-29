package agent

// Config is a subset of the main application config that agent needs
type Config struct {
	Completions struct {
		APIKey      string
		Provider    string
		DefaultHost string
	}
	// Additional fields can be added as needed
}
