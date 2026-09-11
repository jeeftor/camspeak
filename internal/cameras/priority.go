package cameras

import (
	"context"
	"sync"
)

// AudioPriority reserves a camera for triggered audio while allowing interruptible
// AirPlay sessions between triggers. Its zero value is ready for use.
type AudioPriority struct {
	mu         sync.Mutex
	foreground int
	generation uint64
	background context.CancelFunc
}

// Reserve interrupts AirPlay and holds priority through preparation and playback.
// Every reservation has its own idempotent release, including replaced operations.
func (p *AudioPriority) Reserve() func() {
	p.mu.Lock()
	p.foreground++
	p.generation++
	cancel := p.background
	p.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return sync.OnceFunc(func() {
		p.mu.Lock()
		p.foreground--
		p.generation++
		p.mu.Unlock()
	})
}

// State identifies fresh AirPlay data and whether a trigger currently owns audio.
func (p *AudioPriority) State() (generation uint64, blocked bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.generation, p.foreground != 0
}

// BeginAirPlay admits only fresh audio when no triggered action has priority.
// The returned context is canceled immediately by the next reservation.
func (p *AudioPriority) BeginAirPlay(
	ctx context.Context,
	generation uint64,
) (context.Context, func(), bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.foreground != 0 || p.generation != generation || p.background != nil || ctx.Err() != nil {
		return nil, nil, false
	}
	ctx, cancel := context.WithCancel(ctx)
	p.background = cancel
	return ctx, sync.OnceFunc(func() {
		cancel()
		p.mu.Lock()
		p.background = nil
		p.mu.Unlock()
	}), true
}

// PrioritySpeaker exposes arbitration on backends that can receive AirPlay.
type PrioritySpeaker interface {
	AudioPriority() *AudioPriority
}

// ReserveTriggered prevents AirPlay reconnecting while an action prepares audio.
func ReserveTriggered(speaker Speaker) func() {
	if prioritized, ok := speaker.(PrioritySpeaker); ok {
		return prioritized.AudioPriority().Reserve()
	}
	return func() {}
}
