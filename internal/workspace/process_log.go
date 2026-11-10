package workspace

import (
	"bytes"
	"os"
	"strings"
	"sync"
)

const processLogLimit = 256 * 1024

// processLog bounds both memory and disk; development services keep the newest output.
type processLog struct {
	mu        sync.Mutex
	buffer    bytes.Buffer
	file      *os.File
	tail      bool
	truncated bool
}

func (l *processLog) Write(data []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := len(data)
	if l.tail && l.buffer.Len()+n > processLogLimit {
		if n >= processLogLimit {
			l.buffer.Reset()
			l.buffer.Write(data[n-processLogLimit:])
		} else {
			l.buffer.Next(l.buffer.Len() + n - processLogLimit)
			l.buffer.Write(data)
		}
		if err := l.file.Truncate(0); err != nil {
			return 0, err
		}
		if _, err := l.file.Seek(0, 0); err != nil {
			return 0, err
		}
		if _, err := l.file.Write(l.buffer.Bytes()); err != nil {
			return 0, err
		}
		return n, nil
	}
	remaining := max(0, processLogLimit-l.buffer.Len())
	if len(data) > remaining {
		data = data[:remaining]
	}
	if len(data) > 0 {
		if _, err := l.file.Write(data); err != nil {
			return 0, err
		}
		l.buffer.Write(data)
	}
	if n > remaining && !l.truncated {
		const note = "\n[日志超过 256 KiB，后续输出已省略；任务继续运行]\n"
		if _, err := l.file.WriteString(note); err != nil {
			return 0, err
		}
		l.buffer.WriteString(note)
		l.truncated = true
	}
	return n, nil
}

func (l *processLog) Text() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return strings.ToValidUTF8(l.buffer.String(), "�")
}

func (l *processLog) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.file.Close()
}
