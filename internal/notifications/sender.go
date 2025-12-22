package notifications

import "context"

// Sender delivers notifications to a specific channel implementation.
type Sender interface {
	Send(context.Context, Message) error
}
