package belief

import "testing"

func TestObjectiveKeys(t *testing.T) {
	t.Parallel()

	if got := ObjectiveManifestKey("", "objective-1"); got != "objective/project/default/objective-1/manifest" {
		t.Fatalf("unexpected manifest key %q", got)
	}
	first := ObjectiveSessionKey("project-1", "session with spaces")
	second := ObjectiveSessionKey("project-1", "session with spaces")
	if first == "" || first != second {
		t.Fatalf("expected stable session key, got %q and %q", first, second)
	}
	if got := ObjectiveActivePlanKey("project-1", "objective-1"); got != "objective/project/project-1/objective-1/working/active_plan" {
		t.Fatalf("unexpected active plan key %q", got)
	}
	if got := ObjectiveHandoffKey("project-1", "objective-1"); got != "objective/project/project-1/objective-1/working/handoff" {
		t.Fatalf("unexpected handoff key %q", got)
	}
	if got := ObjectiveOpenQuestionsKey("project-1", "objective-1"); got != "objective/project/project-1/objective-1/working/open_questions" {
		t.Fatalf("unexpected open questions key %q", got)
	}
}
