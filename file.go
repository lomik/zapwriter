package zapwriter

import (
	"fmt"
	"os"
	"sync"
	"syscall"
	"time"
)

const fileCheckInterval = 100 * time.Millisecond

// with external rotate support
type FileOutput struct {
	sync.Mutex
	checkNext time.Time
	f         *os.File
	path      string // filename

	exit     chan interface{}
	exitOnce sync.Once
	exitWg   sync.WaitGroup
}

func newFileOutput(path string) (*FileOutput, error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}

	r := &FileOutput{
		checkNext: time.Now().Add(fileCheckInterval),
		f:         f,
		path:      path,
		exit:      make(chan interface{}),
	}

	r.exitWg.Add(1)
	go func() {
		r.reopenChecker(r.exit)
		r.exitWg.Done()
	}()

	return r, nil
}

func File(path string) (*FileOutput, error) {
	return newFileOutput(path)
}

func (r *FileOutput) reopenChecker(exit chan interface{}) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			r.Lock()
			r.check()
			r.Unlock()
		case <-exit:
			return
		}
	}
}

func (r *FileOutput) reopen() *os.File {
	prev := r.f
	next, err := os.OpenFile(r.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Println(err.Error())
		return r.f
	}

	r.f = next
	prev.Close()
	return r.f
}

func (r *FileOutput) check() {
	now := time.Now()

	if now.Before(r.checkNext) {
		return
	}

	r.checkNext = time.Now().Add(fileCheckInterval)

	fInfo, err := r.f.Stat()
	if err != nil {
		fmt.Println(err.Error())
		return
	}

	fStat, ok := fInfo.Sys().(*syscall.Stat_t)
	if !ok {
		fmt.Println("Not a syscall.Stat_t")
		return
	}

	pInfo, err := os.Stat(r.path)
	if err != nil {
		// file deleted (?)
		r.reopen()
		return
	}

	pStat, ok := pInfo.Sys().(*syscall.Stat_t)
	if !ok {
		fmt.Println("Not a syscall.Stat_t")
		return
	}

	if fStat.Ino != pStat.Ino {
		// file on disk changed
		r.reopen()
		return
	}
}

func (r *FileOutput) Write(p []byte) (n int, err error) {
	r.Lock()
	r.check()
	n, err = r.f.Write(p)
	r.Unlock()
	return
}

func (r *FileOutput) Sync() (err error) {
	r.Lock()
	r.check()
	err = r.f.Sync()
	r.Unlock()
	return
}

func (r *FileOutput) Close() (err error) {
	r.exitOnce.Do(func() {
		close(r.exit)
	})
	r.exitWg.Wait()
	err = r.f.Close()
	return
}
