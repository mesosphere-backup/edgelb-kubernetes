package config

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/mesosphere/dcos-edge-lb/apiserver/util"
)

const (
	chunkSize = 64000
)

// FileChangeHandleFn is the file change callback function
type FileChangeHandleFn func(context.Context) error

// Watcher is an interface that exists for testing purposes
type Watcher interface {
	Run(context.Context) error
	String() string
}

// fileWatcher a wrapper for fsnotify.Watcher to handle file changes
type fileWatcher struct {
	*fsnotify.Watcher

	onFileChange FileChangeHandleFn
	configDir    string
	re           *regexp.Regexp
	globPattern  string
}

// MakeFileWatcherFn returns a function that creates a file watcher configured with a work directory
func MakeFileWatcherFn(configDir string) func(string, string, FileChangeHandleFn) (Watcher, error) {
	return func(pattern, globPattern string, onFileChange FileChangeHandleFn) (Watcher, error) {
		return NewFileWatcher(
			configDir,
			fmt.Sprintf(`^%s/%s$`, configDir, pattern),
			fmt.Sprintf("%s/%s", configDir, globPattern),
			onFileChange,
		)
	}
}

// NewFileWatcher creates a new FSNotify watcher and handler
func NewFileWatcher(configDir, pattern, globPattern string, onFileChange FileChangeHandleFn) (Watcher, error) {
	// XXX Instead of watching the entire configDir, should we instead
	// watch only the specific files we're interested in?

	logger.Infof("creating new file watcher matching %s glob pattern and %s regex pattern", globPattern, pattern)

	re := regexp.MustCompile(pattern)
	w := &fileWatcher{
		onFileChange: onFileChange,
		configDir:    configDir,
		re:           re,
		globPattern:  globPattern,
	}
	var watchErr error
	w.Watcher, watchErr = fsnotify.NewWatcher()
	if watchErr != nil {
		return nil, fmt.Errorf("error creating new watcher: %s", watchErr)
	}
	if err := w.Watcher.Add(configDir); err != nil {
		return nil, fmt.Errorf("error adding %s to watcher: %s", configDir, err)
	}
	return w, nil
}

// String returns what is being watched
func (w *fileWatcher) String() string {
	return fmt.Sprintf("configDir: %s, regex: %s, glob: %s", w.configDir, w.re.String(), w.globPattern)
}

// Run watches files and only executes callback if contents change
func (w *fileWatcher) Run(ctx context.Context) error {
	if err := w.doRun(ctx); err != nil {
		return fmt.Errorf(util.MsgWithClose(err.Error(), w.Watcher))
	}
	return w.Watcher.Close()
}

func (w *fileWatcher) doRun(origCtx context.Context) error {
	ctx, cancel := context.WithCancel(origCtx)
	// This extra cancel is just to pass the linter.
	defer cancel()

	// Check all files before watching to catch changes before starting
	if err := w.checkAllFiles(ctx); err != nil {
		return err
	}

	eventC := make(chan string)
	errC := make(chan error)
	go w.handleEvents(ctx, eventC, errC)

	for {
		select {
		case err := <-errC:
			logger.Info("fileWatcher Run errC")
			return err
		case event := <-w.Watcher.Events:
			if event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create {
				select {
				case err := <-errC:
					// This is here so we don't hang trying to write to eventC
					logger.Info("fileWatcher Run event errC")
					return err
				case eventC <- event.Name:
					// noop
				}
			}
		case watchErr := <-w.Watcher.Errors:
			// XXX Write a test that checks that an error here won't cause
			// any goroutines in the watcher to hang.

			// Cancel here so that the final result comes out of the errC
			cancel()
			handleErr := <-errC
			// Even if the error is nil, fmt will still print a thing
			return fmt.Errorf("fileWatcher Run: (watchErr: %+v :: handleErr: %+v)", watchErr, handleErr)
		}
	}
}

func (w *fileWatcher) handleEvents(ctx context.Context, eventC chan string, errC chan error) {
	// Callers of handleEvents must always listen to errC before returning so
	// that this doesn't hang.
	errC <- w.runHandleEvents(ctx, eventC)
}

func (w *fileWatcher) runHandleEvents(ctx context.Context, eventC chan string) error {
	// XXX write a unit test for this event handler logic

	eventMut := &sync.Mutex{}
	events := make(map[string]struct{})

	// This has a queue size 1 so it can have at most 1 signal for the
	// consumer to know that something has changed since it last read.
	signalC := make(chan struct{}, 1)

	errC := make(chan error)
	go w.consumeEvents(ctx, signalC, eventMut, events, errC)

	for {
		select {
		case err := <-errC:
			logger.Info("fileWatcher handleEvents errC")
			return err
		case name := <-eventC:
			eventMut.Lock()
			events[name] = struct{}{}
			select {
			case signalC <- struct{}{}:
				// noop
			default:
				// noop for nonblocking
			}
			eventMut.Unlock()
		}
	}
}

func (w *fileWatcher) consumeEvents(ctx context.Context, signalC chan struct{}, eventMut sync.Locker, origEvents map[string]struct{}, errC chan error) {
	// Callers of handleEvents must always listen to errC before returning so
	// that this doesn't hang.
	errC <- w.runConsumeEvents(ctx, signalC, eventMut, origEvents)
}

func (w *fileWatcher) runConsumeEvents(ctx context.Context, signalC chan struct{}, eventMut sync.Locker, origEvents map[string]struct{}) error {
	for {
		select {
		case <-ctx.Done():
			logger.Info("fileWatcher consumeEvents done")
			return nil
		case <-signalC:
			events := make(map[string]struct{})
			eventMut.Lock()
			for k, v := range origEvents {
				events[k] = v
				delete(origEvents, k)
			}
			eventMut.Unlock()

			for name := range events {
				if err := w.checkFile(ctx, name); err != nil {
					return err
				}
			}
		}
	}
}

func (w *fileWatcher) checkAllFiles(ctx context.Context) error {
	matches, matchErr := filepath.Glob(w.globPattern)
	if matchErr != nil {
		return fmt.Errorf("error copying rendered config: %s", matchErr)
	}
	for _, filename := range matches {
		if err := w.checkFile(ctx, filename); err != nil {
			return err
		}
	}
	return nil
}

func (w *fileWatcher) checkFile(ctx context.Context, filename string) error {
	if !w.re.MatchString(filename) {
		return nil
	}
	logger.Infof("watcher got file event for: %v", filename)
	copiedFile := fmt.Sprintf("%s.bak", filename)
	if _, err := os.Stat(copiedFile); err == nil {
		if eq, err := filesEqual(filename, copiedFile); err != nil {
			return err
		} else if eq {
			logger.Infof("files were equal, ignoring")
			return nil
		}
	}
	cmd := exec.Command("cp", filename, copiedFile)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error copying rendered config: %s", err)
	}
	logger.Infof("files were not equal, updating")
	return w.onFileChange(ctx)
}

func filesEqual(file1, file2 string) (bool, error) {
	pass, f1, f2, err := filesEqualPrecheck(file1, file2)
	if err != nil {
		return false, err
	}
	if !pass {
		return false, nil
	}

	for {
		b1 := make([]byte, chunkSize)
		_, err1 := f1.Read(b1)

		b2 := make([]byte, chunkSize)
		_, err2 := f2.Read(b2)

		if err1 != nil || err2 != nil {
			if err1 == io.EOF && err2 == io.EOF {
				return true, nil
			} else if err1 == io.EOF || err2 == io.EOF {
				return false, nil
			} else {
				return false, fmt.Errorf("(%s, %s)", err1, err2)
			}
		}

		if !bytes.Equal(b1, b2) {
			return false, nil
		}
	}
}

func filesEqualPrecheck(file1, file2 string) (bool, *os.File, *os.File, error) {
	f1s, err := os.Stat(file1)
	if err != nil {
		return false, nil, nil, err
	}
	f2s, err := os.Stat(file2)
	if err != nil {
		return false, nil, nil, err
	}

	if f1s.Size() != f2s.Size() {
		return false, nil, nil, nil
	}

	f1, err := os.Open(file1)
	if err != nil {
		return false, nil, nil, err
	}

	f2, err := os.Open(file2)
	if err != nil {
		return false, nil, nil, err
	}

	return true, f1, f2, nil
}
