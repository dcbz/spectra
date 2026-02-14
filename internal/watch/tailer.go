package watch

import (
	"context"
	"fmt"
	"sync"

	"github.com/nxadm/tail"
)

// LogEvent represents a single line read from a log file.
type LogEvent struct {
	Path string
	Line string
	Err  error
}

// TailFiles streams log lines from multiple files.
func TailFiles(ctx context.Context, files []string) (<-chan LogEvent, error) {
	if len(files) == 0 {
		return nil, fmt.Errorf("no files provided")
	}

	out := make(chan LogEvent)
	wg := &sync.WaitGroup{}
	wg.Add(len(files))

	for _, file := range files {
		cfg := tail.Config{Follow: true, ReOpen: true, Logger: tail.DiscardingLogger, MustExist: true}
		t, err := tail.TailFile(file, cfg)
		if err != nil {
			return nil, fmt.Errorf("tail %s: %w", file, err)
		}

		go func(p string, tails *tail.Tail) {
			defer wg.Done()
			defer tails.Cleanup()
			for {
				select {
				case <-ctx.Done():
					return
				case line, ok := <-tails.Lines:
					if !ok {
						return
					}
					if line.Err != nil {
						out <- LogEvent{Path: p, Err: line.Err}
						continue
					}
					out <- LogEvent{Path: p, Line: line.Text}
				}
			}
		}(file, t)
	}

	go func() {
		defer close(out)
		wg.Wait()
	}()

	return out, nil
}
