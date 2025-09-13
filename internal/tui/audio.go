package tui

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"

	"intelligence.dev/internal/observability"
)

// playAudioFile plays an audio file using the system's default audio player
func playAudioFile(filePath string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin": // macOS
		cmd = exec.Command("afplay", filePath)
	case "linux":
		// Try common Linux audio players
		if _, err := exec.LookPath("paplay"); err == nil {
			cmd = exec.Command("paplay", filePath)
		} else if _, err := exec.LookPath("aplay"); err == nil {
			cmd = exec.Command("aplay", filePath)
		} else if _, err := exec.LookPath("mpg123"); err == nil {
			cmd = exec.Command("mpg123", filePath)
		} else {
			return fmt.Errorf("no audio player found (tried paplay, aplay, mpg123)")
		}
	case "windows":
		// Windows PowerShell command to play audio
		cmd = exec.Command("powershell", "-c", "(New-Object Media.SoundPlayer '"+filePath+"').PlaySync();")
	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}

	return cmd.Run()
}

// handleTTSToolResult processes TTS tool results and plays the audio
func (m *Model) handleTTSToolResult(toolName string, args []byte, result []byte) {
	if toolName != "text_to_speech" {
		return
	}

	// Parse the tool result to extract file path
	var response map[string]interface{}
	if err := json.Unmarshal(result, &response); err != nil {
		observability.LoggerWithTrace(m.ctx).Error().Err(err).Msg("failed to parse TTS response")
		return
	}

	// Check if the response contains a file path
	filePath, ok := response["file_path"].(string)
	if !ok || filePath == "" {
		observability.LoggerWithTrace(m.ctx).Debug().Msg("TTS response does not contain file_path")
		return
	}

	// Play the audio file in a goroutine so it doesn't block the UI
	go func() {
		if err := playAudioFile(filePath); err != nil {
			observability.LoggerWithTrace(m.ctx).Error().Err(err).Str("file", filePath).Msg("failed to play audio file")
		} else {
			observability.LoggerWithTrace(m.ctx).Info().Str("file", filePath).Msg("audio played successfully")
		}
	}()
}
