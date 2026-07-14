package onboarding

import "fmt"

func PromptIDs(userID int64, name, version string) (string, string) {
	promptID := name
	if userID != 0 {
		promptID = fmt.Sprintf("%s-%d", name, userID)
	}
	return promptID, promptID + "-" + version
}
