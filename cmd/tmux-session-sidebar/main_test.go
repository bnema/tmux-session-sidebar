package main

import (
	"context"
	"io"
	"os"
	"syscall"
	"testing"

	"github.com/bnema/tmux-session-sidebar/internal/app"
)

func TestRunUsesSignalAwareContext(t *testing.T) {
	oldSignalContext := signalContext
	oldRunApp := runApp
	oldNewRouter := newRouter
	defer func() {
		signalContext = oldSignalContext
		runApp = oldRunApp
		newRouter = oldNewRouter
	}()

	var gotSignals []os.Signal
	var gotContext context.Context
	signalContext = func(parent context.Context, signals ...os.Signal) (context.Context, context.CancelFunc) {
		gotSignals = append([]os.Signal(nil), signals...)
		return context.WithCancel(parent)
	}
	runApp = func(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer, router app.Router) int {
		gotContext = ctx //nolint:fatcontext // test captures the context only for assertions.
		return 0
	}
	newRouter = func() app.Router { return nil }

	if exitCode := run([]string{"daemon", "serve"}, io.Discard, io.Discard); exitCode != 0 {
		t.Fatalf("run() exitCode = %d, want 0", exitCode)
	}
	if gotContext == nil {
		t.Fatal("run() did not pass a context to app.Run")
	}
	if len(gotSignals) != 2 || gotSignals[0] != os.Interrupt || gotSignals[1] != syscall.SIGTERM {
		t.Fatalf("signals = %#v, want [os.Interrupt syscall.SIGTERM]", gotSignals)
	}
}
