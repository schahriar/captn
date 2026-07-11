package tui

import (
	"context"
	"sync"
)

type StatusType string

const (
	StatusTypeProgress StatusType = "progress"
)

type statusContextKeyType string

const statusContextKey statusContextKeyType = "statusContext"

type StatusProvider struct {
	overlay *Overlay

	mu    sync.Mutex
	timer *Timer
	tasks []string
}

func NewStatusProvider(overlay *Overlay) *StatusProvider {
	return &StatusProvider{
		overlay: overlay,
	}
}

func WithStatusProvider(ctx context.Context, overlay *Overlay) context.Context {
	provider := NewStatusProvider(overlay)

	return context.WithValue(ctx, statusContextKey, provider)
}

func ReportStatus(ctx context.Context, t StatusType, status string) {
	provider, ok := GetStatusProvider(ctx)

	if !ok {
		return
	}

	provider.ReportStatus(t, status)
}

func PushStatusTask(ctx context.Context, t StatusType, status string) func() {
	provider, ok := GetStatusProvider(ctx)

	if !ok {
		return func() {}
	}

	return provider.PushTask(t, status)
}

func GetStatusProvider(ctx context.Context) (*StatusProvider, bool) {
	provider, ok := ctx.Value(statusContextKey).(*StatusProvider)

	if !ok {
		return nil, false
	}

	return provider, true
}

func (p *StatusProvider) PushTask(t StatusType, status string) func() {
	if p == nil {
		return func() {}
	}

	p.mu.Lock()
	if p.timer == nil {
		p.timer = NewTimer()
		p.timer.Start()
	}
	timer := p.timer
	p.tasks = append(p.tasks, status)
	p.mu.Unlock()

	p.overlay.Show()
	p.report(status, timer)

	var once sync.Once
	return func() {
		once.Do(p.popTask)
	}
}

func (p *StatusProvider) popTask() {
	p.mu.Lock()
	if n := len(p.tasks); n > 0 {
		p.tasks = p.tasks[:n-1]
	}

	if n := len(p.tasks); n > 0 {
		status := p.tasks[n-1]
		timer := p.timer
		p.mu.Unlock()
		p.report(status, timer)
		return
	}

	if p.timer != nil {
		p.timer.Stop()
		p.timer = nil
	}
	p.mu.Unlock()

	p.overlay.Hide()
}

func (p *StatusProvider) ReportStatus(t StatusType, status string) {
	if p == nil {
		return
	}

	p.mu.Lock()
	timer := p.timer
	p.mu.Unlock()

	p.report(status, timer)
}

func (p *StatusProvider) report(status string, timer *Timer) {
	sections := []Section{Text(Bold(" ⚓ captn") + status)}
	if timer != nil {
		sections = append(sections, timer)
	}

	p.overlay.SetStatus(
		Decorate(
			Group(sections...),
			Shimmer,
		),
	)
}
