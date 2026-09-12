package logger

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// fileWriter is an io.Writer that appends to a log file whose name carries the
// current date, rotating to a new file when the date changes. Writes are
// buffered and flushed periodically. It uses only the standard library.
type fileWriter struct {
	baseName string
	dir      string
	ext      string

	mu       sync.Mutex
	file     *os.File
	buf      *bufio.Writer
	filename string
	bufSize  int

	lastSuffix atomic.Value // string
	stopFlush  chan struct{}
	flushDone  sync.WaitGroup
}

// newFileWriter opens a rotated log file derived from fileName. bufSizeKB and
// flushInterval (seconds) tune buffering; non-positive values use defaults.
func newFileWriter(fileName string, bufSizeKB, flushInterval int) (*fileWriter, error) {
	dir := filepath.Dir(fileName)
	base := filepath.Base(fileName)
	ext := filepath.Ext(base)
	name := base[:len(base)-len(ext)]
	if bufSizeKB <= 0 {
		bufSizeKB = 64
	}

	suffix := time.Now().Format(time.DateOnly)
	fw := &fileWriter{
		baseName:  name,
		dir:       dir,
		ext:       ext,
		filename:  filepath.Join(dir, fmt.Sprintf("%s-%s%s", name, suffix, ext)),
		bufSize:   bufSizeKB * 1024,
		stopFlush: make(chan struct{}),
	}
	fw.lastSuffix.Store(suffix)

	if err := fw.openLogFile(); err != nil {
		return nil, err
	}

	fw.flushDone.Add(1)
	go fw.periodicFlush(flushInterval)

	return fw, nil
}

func (fw *fileWriter) periodicFlush(interval int) {
	if interval <= 0 {
		interval = 1
	}
	defer fw.flushDone.Done()

	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			fw.mu.Lock()
			if fw.buf != nil {
				_ = fw.buf.Flush()
			}
			fw.mu.Unlock()
		case <-fw.stopFlush:
			return
		}
	}
}

func (fw *fileWriter) Write(p []byte) (int, error) {
	currentSuffix := time.Now().Format(time.DateOnly)
	if currentSuffix != fw.lastSuffix.Load().(string) {
		fw.mu.Lock()
		if currentSuffix != fw.lastSuffix.Load().(string) {
			if fw.buf != nil {
				_ = fw.buf.Flush()
			}
			fw.filename = filepath.Join(fw.dir,
				fmt.Sprintf("%s-%s%s", fw.baseName, currentSuffix, fw.ext))
			if err := fw.openLogFile(); err != nil {
				fw.mu.Unlock()
				return 0, fmt.Errorf("reopen log file: %w", err)
			}
			fw.lastSuffix.Store(currentSuffix)
		}
		fw.mu.Unlock()
	}

	fw.mu.Lock()
	n, err := fw.buf.Write(p)
	fw.mu.Unlock()
	if err != nil {
		return n, fmt.Errorf("write log: %w", err)
	}
	return n, nil
}

func (fw *fileWriter) openLogFile() error {
	if fw.buf != nil {
		_ = fw.buf.Flush()
	}
	if fw.file != nil {
		_ = fw.file.Close()
	}

	if err := os.MkdirAll(fw.dir, 0o755); err != nil {
		return fmt.Errorf("create log dir: %w", err)
	}

	file, err := os.OpenFile(fw.filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}

	fw.file = file
	fw.buf = bufio.NewWriterSize(file, fw.bufSize)
	return nil
}

// Flush writes buffered data and syncs the file to disk.
func (fw *fileWriter) Flush() error {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	if fw.buf != nil {
		if err := fw.buf.Flush(); err != nil {
			return err
		}
	}
	if fw.file != nil {
		return fw.file.Sync()
	}
	return nil
}

// Close stops periodic flushing and closes the file.
func (fw *fileWriter) Close() error {
	close(fw.stopFlush)
	fw.flushDone.Wait()

	fw.mu.Lock()
	defer fw.mu.Unlock()

	var err error
	if fw.buf != nil {
		err = fw.buf.Flush()
	}
	if fw.file != nil {
		if cerr := fw.file.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}
	return err
}
