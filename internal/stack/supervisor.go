package stack

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// Run starts the named services and stops them when the context is canceled or a service exits.
func Run(ctx context.Context, names ...string) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	exits := make(chan error, len(names))
	started := 0
	var result error
	for _, name := range names {
		cmd := exec.CommandContext(ctx, name)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
		cmd.WaitDelay = 5 * time.Second
		if err := cmd.Start(); err != nil {
			result = fmt.Errorf("start %s: %w", name, err)
			break
		}
		started++
		go func() {
			if err := cmd.Wait(); err != nil {
				exits <- fmt.Errorf("%s exited: %w", name, err)
			} else {
				exits <- fmt.Errorf("%s exited unexpectedly", name)
			}
		}()
	}

	if result == nil {
		select {
		case <-ctx.Done():
		case result = <-exits:
			started--
		}
	}
	cancel()
	for range started {
		<-exits
	}
	return result
}
