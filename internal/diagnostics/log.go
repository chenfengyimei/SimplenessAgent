// Package diagnostics provides small, local, privacy-conscious diagnostic logs.
package diagnostics

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const maxMessageLength = 4000

type Entry struct {
	Timestamp time.Time         `json:"timestamp"`
	Level     string            `json:"level"`
	Component string            `json:"component"`
	Message   string            `json:"message"`
	Fields    map[string]string `json:"fields,omitempty"`
}

type Logger struct {
	root string
	mu   sync.Mutex
}

func Open(root string) (*Logger, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	return &Logger{root: root}, nil
}

func (l *Logger) Info(component, message string, fields map[string]string) {
	l.write("INFO", component, message, fields)
}
func (l *Logger) Error(component, message string, fields map[string]string) {
	l.write("ERROR", component, message, fields)
}

func (l *Logger) write(level, component, message string, fields map[string]string) {
	if l == nil {
		return
	}
	entry := Entry{Timestamp: time.Now().UTC(), Level: level, Component: trim(component), Message: trim(message), Fields: sanitize(fields)}
	encoded, err := json.Marshal(entry)
	if err != nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	file, err := os.OpenFile(filepath.Join(l.root, "desktop-"+time.Now().UTC().Format("2006-01-02")+".jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	_, _ = file.Write(append(encoded, '\n'))
	_ = file.Close()
}

func (l *Logger) Query(limit int) ([]Entry, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	if l == nil {
		return []Entry{}, nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	files, err := filepath.Glob(filepath.Join(l.root, "desktop-*.jsonl"))
	if err != nil {
		return nil, err
	}
	sort.Sort(sort.Reverse(sort.StringSlice(files)))
	items := make([]Entry, 0, limit)
	for _, name := range files {
		file, openErr := os.Open(name)
		if openErr != nil {
			continue
		}
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 1024), 128*1024)
		batch := []Entry{}
		for scanner.Scan() {
			var entry Entry
			if json.Unmarshal(scanner.Bytes(), &entry) == nil {
				batch = append(batch, entry)
			}
		}
		_ = file.Close()
		for index := len(batch) - 1; index >= 0 && len(items) < limit; index-- {
			items = append(items, batch[index])
		}
		if len(items) >= limit {
			break
		}
	}
	return items, nil
}

func trim(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > maxMessageLength {
		return value[:maxMessageLength] + "…"
	}
	return value
}

func sanitize(fields map[string]string) map[string]string {
	if len(fields) == 0 {
		return nil
	}
	result := make(map[string]string, len(fields))
	for key, value := range fields {
		lower := strings.ToLower(key)
		if strings.Contains(lower, "key") || strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "password") {
			result[key] = "[REDACTED]"
			continue
		}
		result[key] = trim(value)
	}
	return result
}
