package wharf

import "context"

// Notifier sends an out-of-band ping. SSH is pull-only — nobody re-runs `ssh`
// unprompted — so a notifier is the retention layer: tell people a new issue is
// out, or that someone just joined the room.
//
// Implementations live in their own packages (notify/telegram, notify/webhook)
// and use only the standard library, so they add no dependencies to the core.
type Notifier interface {
	Notify(ctx context.Context, n Notification) error
}

// Notification is one ping. User is set when the ping concerns a specific
// person (e.g. a connect notification); it may be the zero value otherwise.
type Notification struct {
	Title string
	Body  string
	User  User
}
