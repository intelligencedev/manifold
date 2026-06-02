package durable

func normalizeEventListLimit(limit int) int {
	switch {
	case limit <= 0:
		return DefaultEventListLimit
	case limit > MaxEventListLimit:
		return MaxEventListLimit
	default:
		return limit
	}
}

func withEventPageEvents(page EventPage, events []Event) EventPage {
	page.Events = events
	if len(events) > 0 {
		page.FirstSequence = events[0].Sequence
		page.LastSequence = events[len(events)-1].Sequence
	}
	return page
}
