package lease

import (
	"errors"
	"testing"
)

func TestClassifySyncAckError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{name: "timeout", err: errors.New("context deadline exceeded"), want: "ack_timeout"},
		{name: "refused", err: errors.New("connection refused"), want: "ack_conn_refused"},
		{name: "reject", err: errors.New("partner rejected commit"), want: "ack_rejected"},
		{name: "protocol", err: errors.New("peer ack phase mismatch"), want: "ack_protocol_error"},
		{name: "io", err: errors.New("broken pipe"), want: "ack_io_error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifySyncAckError(tc.err); got != tc.want {
				t.Fatalf("classifySyncAckError() = %q, want %q", got, tc.want)
			}
		})
	}
}
