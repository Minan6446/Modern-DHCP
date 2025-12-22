package events

import "context"

// MultiPublisher fans out events to multiple downstream publishers.
type MultiPublisher struct {
	publishers []PolicyPublisher
}

// NewMultiPublisher creates a new fan-out publisher.
func NewMultiPublisher(pubs ...PolicyPublisher) *MultiPublisher {
	return &MultiPublisher{publishers: pubs}
}

func (m *MultiPublisher) Publish(ctx context.Context, evt PolicyEvent) error {
	for _, pub := range m.publishers {
		if pub == nil {
			continue
		}
		if err := pub.Publish(ctx, evt); err != nil {
			return err
		}
	}
	return nil
}

func (m *MultiPublisher) Close(ctx context.Context) error {
	for _, pub := range m.publishers {
		if pub == nil {
			continue
		}
		if err := pub.Close(ctx); err != nil {
			return err
		}
	}
	return nil
}
