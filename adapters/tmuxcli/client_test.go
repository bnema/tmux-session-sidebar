package tmuxcli

import (
	"context"
	"testing"

	"github.com/bnema/tmux-session-sidebar/ports"
	"github.com/bnema/tmux-session-sidebar/ports/mocks"
)

func TestListSessionsParsesTmuxRows(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want []ports.TmuxSessionSnapshot
	}{
		{name: "one row", out: "$1\talpha\t2\t1\n", want: []ports.TmuxSessionSnapshot{{ID: "$1", Name: "alpha", WindowCount: 2, AttachedCount: 1}}},
		{name: "skips malformed", out: "bad\n$2\tbeta\t1\t0\n", want: []ports.TmuxSessionSnapshot{{ID: "$2", Name: "beta", WindowCount: 1, AttachedCount: 0}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			process := mocks.NewMockProcessPort(t)
			process.EXPECT().Exec(ctx, "tmux", []string{"list-sessions", "-F", "#{session_id}\t#{session_name}\t#{session_windows}\t#{session_attached}"}).Return(ports.Result{Stdout: tt.out}, nil)

			got, err := (Client{Process: process}).ListSessions(ctx)
			if err != nil {
				t.Fatalf("ListSessions error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("len = %d, want %d", len(got), len(tt.want))
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Fatalf("session[%d] = %#v, want %#v", i, got[i], tt.want[i])
				}
			}
		})
	}
}
