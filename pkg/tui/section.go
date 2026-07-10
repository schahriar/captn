package tui

import (
	"fmt"
	"sync"
	"time"
)

// Section is a piece of the status bar, rendered on every frame.
type Section interface {
	Render() string
}

// Text is a static Section.
type Text string

func (t Text) Render() string { return string(t) }

// SectionFunc adapts a function into a Section evaluated on every render.
type SectionFunc func() string

func (f SectionFunc) Render() string { return f() }

func Group(sections ...Section) Section {
	return SectionFunc(func() string {
		return renderSections(sections)
	})
}

func Decorate(s Section, decorators ...func(string) string) Section {
	return SectionFunc(func() string {
		out := s.Render()
		for _, d := range decorators {
			out = d(out)
		}
		return out
	})
}

type Timer struct {
	mu      sync.Mutex
	started time.Time
	elapsed time.Duration
	running bool
}

func NewTimer() *Timer {
	// banstructlit:ignore
	return &Timer{}
}

func (t *Timer) Start() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.running {
		return
	}
	t.started = time.Now()
	t.running = true
}

func (t *Timer) Stop() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.running {
		return
	}
	t.elapsed += time.Since(t.started)
	t.running = false
}

func (t *Timer) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.elapsed = 0
	if t.running {
		t.started = time.Now()
	}
}

func (t *Timer) Render() string {
	t.mu.Lock()
	d := t.elapsed
	if t.running {
		d += time.Since(t.started)
	}
	t.mu.Unlock()

	mins := int(d / time.Minute)
	secs := int((d % time.Minute) / time.Second)
	return fmt.Sprintf("%02d:%02ds", mins, secs)
}
