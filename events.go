package main

// The events amenbo fires. All eleven are subscribed to in the manifest, because what a plugin
// subscribes to is fixed when it is installed and what it reports is not: the choice belongs in
// a setting the user can change afterwards, so everything arrives here and the choosing is done
// below.
const (
	eventTaskCreated       = "task.created"
	eventTaskStatusChanged = "task.status_changed"
	eventTaskDone          = "task.done"
	eventTaskRejected      = "task.rejected"
	eventTaskAssigned      = "task.assigned"
	eventTaskMoved         = "task.moved"
	eventTaskDeleted       = "task.deleted"
	eventDecisionAccepted  = "decision.accepted"
	eventDecisionRejected  = "decision.rejected"
	eventCommentAdded      = "comment.added"
	eventCommentRemoved    = "comment.removed"
)

// reportableEvents is everything the user may pick from, in the order the manifest offers them.
var reportableEvents = []string{
	eventTaskCreated,
	eventTaskStatusChanged,
	eventTaskDone,
	eventTaskRejected,
	eventTaskAssigned,
	eventTaskMoved,
	eventTaskDeleted,
	eventDecisionAccepted,
	eventDecisionRejected,
	eventCommentAdded,
	eventCommentRemoved,
}

// defaultEvents is what is reported by a user who has not chosen. It is the outline of what
// happened while they were away — work appearing, moving, and ending one way or the other —
// rather than every write in the project.
var defaultEvents = []string{
	eventTaskCreated,
	eventTaskStatusChanged,
	eventTaskDone,
	eventTaskRejected,
}

// selectedEvents is the set of events to report.
//
// amenbo sends the setting on every event, and fills in the manifest's default itself when the
// user has not chosen — so the four are named in the payload rather than assumed here, and the
// list is simply taken at its word. Two things follow from that. An empty list is the user
// having picked none of the candidates (which amenbo's `none` is stored and delivered as), and
// it stays empty: the plugin is switched on and reports nothing, rather than being handed back
// the default it was just told not to send. And a name that is not an event matches no event,
// which is all the checking a name needs — amenbo refuses to store a candidate the manifest does
// not offer, so one can only come from a payload written by hand.
//
// The setting missing altogether is a payload written by hand too, and there the default four
// are what a plugin asked to report something reports.
func selectedEvents(cfg map[string]any) map[string]bool {
	if _, chosen := cfg[keyEvents]; !chosen {
		return eventSet(defaultEvents)
	}
	return eventSet(splitList(configValue(cfg, keyEvents)))
}

// eventSet turns a list of event names into the set a lookup is done against.
func eventSet(names []string) map[string]bool {
	set := make(map[string]bool, len(names))
	for _, name := range names {
		set[name] = true
	}
	return set
}
