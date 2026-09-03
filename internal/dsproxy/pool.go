package dsproxy

import (
	"context"
	"errors"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

// Session pool (async mode).
//
// With history disabled, every completion needs a throwaway chat session.
// The synchronous flow creates one per request, so public traffic pays a
// session-creation round trip before any model output starts. The session
// pool removes that cost: it keeps a standing batch of ready sessions
// (default 5) created ahead of time, hands them out instantly, and the
// moment a request's response has been fully written and processed, the
// consumed session is deleted upstream and a replacement is created to
// refill the batch — so the account never accumulates garbage and the
// batch stays at full strength while the app runs.
//
// Only the history=false path draws from the pool; history=true keeps its
// own dedicated rotating session, and --sync-mode restores the legacy
// per-request flow entirely.

var (
	// ErrPoolClosing is returned by Acquire once Shutdown has begun.
	ErrPoolClosing = errors.New("session pool is shutting down")
	// ErrPoolTimeout is returned by Acquire when no pooled session became
	// available within the configured wait window.
	ErrPoolTimeout = errors.New("timed out waiting for a pooled session")
)

const (
	// defaultPoolSize is the standing batch of pre-made ready sessions.
	defaultPoolSize = 5
	// defaultPoolWait bounds how long a completion request waits for a
	// pooled session before creating one directly (see SESSION_ACQUIRE_TIMEOUT).
	defaultPoolWait = 10 * time.Second
	// poolOpTimeout bounds one upstream create/delete call.
	poolOpTimeout = 30 * time.Second
	// poolCreateBackoffStart / poolCreateBackoffMax shape the retry delay
	// when session creation fails (Cloudflare, rate limits, network...).
	poolCreateBackoffStart = 1 * time.Second
	poolCreateBackoffMax   = 15 * time.Second
	// poolDrainWait bounds how long Shutdown waits for in-flight
	// retire/refill operations before reporting leftovers.
	poolDrainWait = 20 * time.Second
)

// SessionBackend is the slice of DeepSeekAPI the pool needs. *DeepSeekAPI
// satisfies it implicitly; tests substitute a stub.
type SessionBackend interface {
	CreateChatSession(ctx context.Context) (string, error)
	DeleteChatSession(ctx context.Context, sessionIDs ...string) error
}

// lazyBackend resolves the API through ProxyServer.getAPI so pool warmup can
// start before/without a token and pick the client up as soon as it exists.
type lazyBackend struct{ s *ProxyServer }

func (l *lazyBackend) CreateChatSession(ctx context.Context) (string, error) {
	api, err := l.s.getAPI()
	if err != nil {
		return "", err
	}
	return api.CreateChatSession(ctx)
}

func (l *lazyBackend) DeleteChatSession(ctx context.Context, ids ...string) error {
	api, err := l.s.getAPI()
	if err != nil {
		return err
	}
	return api.DeleteChatSession(ctx, ids...)
}

// SessionPool holds the standing batch of ready stateless sessions.
type SessionPool struct {
	log     *log.Logger
	backend SessionBackend
	size    int

	ready chan string // buffered with size; members are unused, clean sessions

	stopOnce sync.Once
	stopCh   chan struct{}
	stopped  atomic.Bool

	wg sync.WaitGroup // outstanding create/delete operations
}

// NewSessionPool builds a pool that keeps size sessions ready. size < 1 is
// clamped to the default batch of 3. Call Start to begin warmup.
func NewSessionPool(logger *log.Logger, backend SessionBackend, size int) *SessionPool {
	if size < 1 {
		size = defaultPoolSize
	}
	return &SessionPool{
		log:     logger,
		backend: backend,
		size:    size,
		ready:   make(chan string, size),
		stopCh:  make(chan struct{}),
	}
}

// Size reports the configured batch size.
func (p *SessionPool) Size() int { return p.size }

// Start launches the warmup goroutines that pre-make the initial batch.
func (p *SessionPool) Start() {
	p.log.Printf("[pool] warming up %d stateless session(s)...", p.size)
	for i := 0; i < p.size; i++ {
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			p.fillSlot("warmup")
		}()
	}
}

// Acquire hands out one ready session, blocking until one is available, ctx
// is done, or wait elapses (wait <= 0 waits indefinitely). The caller MUST
// eventually call Release with the returned ID — even on error paths — so
// the used session is retired and the batch refilled.
func (p *SessionPool) Acquire(ctx context.Context, wait time.Duration) (string, error) {
	var timeout <-chan time.Time
	if wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		timeout = timer.C
	}
	for {
		select {
		case id := <-p.ready:
			debugf("[pool] handed out session %s (%d/%d ready)", id, len(p.ready), p.size)
			return id, nil
		case <-ctx.Done():
			return "", ctx.Err()
		case <-timeout:
			return "", ErrPoolTimeout
		case <-p.stopCh:
			return "", ErrPoolClosing
		}
	}
}

// Release retires a consumed session. It is called only after the response
// has been fully written and processed (or definitively failed), so the
// session is never yanked out from under an in-flight completion: first the
// used session is deleted upstream, then a replacement is created right away
// to fill the gap in the batch. Both steps run in the background.
func (p *SessionPool) Release(sessionID string) {
	if sessionID == "" {
		return
	}
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		p.deleteOne(sessionID, "used")
		if p.stopped.Load() {
			return // shutting down: retire only, don't rebuild the batch
		}
		p.fillSlot("refill")
	}()
}

// Shutdown gracefully clears the pool (the CTRL+C path): refills are
// stopped, every still-pooled session is deleted upstream so nothing is left
// behind on the account, and we briefly wait for in-flight retire/refill
// operations to finish. Sessions already checked out are deleted by their
// request's Release call.
func (p *SessionPool) Shutdown() {
	first := false
	p.stopOnce.Do(func() {
		first = true
		p.stopped.Store(true)
		close(p.stopCh)
	})
	if !first {
		return
	}

	// Collect whatever is still stocked and bulk-delete it.
	var leftover []string
	for {
		select {
		case id := <-p.ready:
			leftover = append(leftover, id)
			continue
		default:
		}
		break
	}
	if len(leftover) > 0 {
		p.log.Printf("[pool] clearing all remaining sessions (%d)...", len(leftover))
		ctx, cancel := context.WithTimeout(context.Background(), poolOpTimeout)
		err := p.backend.DeleteChatSession(ctx, leftover...)
		cancel()
		if err != nil {
			warnf("[pool] warning: failed to clear %d session(s): %v", len(leftover), err)
		} else {
			p.log.Printf("[pool] cleared %d pooled session(s): deleted %v", len(leftover), leftover)
		}
	} else {
		p.log.Printf("[pool] clearing all sessions... none remaining")
	}

	// Wait (bounded) for outstanding creates/deletes to wind down.
	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		p.log.Printf("[pool] all sessions accounted for")
	case <-time.After(poolDrainWait):
		warnf("[pool] warning: some background session operations did not finish within %s", poolDrainWait)
	}
}

// fillSlot creates one session (retrying through transient failures) and
// stocks it, unless shutdown won the race. Runs synchronously; callers wrap
// it in a goroutine where concurrency is wanted.
func (p *SessionPool) fillSlot(reason string) {
	backoff := poolCreateBackoffStart
	loggedOnce := false
	for {
		if p.stopped.Load() {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), poolOpTimeout)
		id, err := p.backend.CreateChatSession(ctx)
		cancel()
		if err != nil {
			if p.stopped.Load() {
				return
			}
			// Log the first failure loudly; repeats stay quiet (debug) so a
			// misconfigured token or a Cloudflare wall doesn't spam the log.
			if !loggedOnce {
				p.log.Printf("[%s] session creation failed (%v); retrying...", reason, err)
				loggedOnce = true
			} else {
				debugf("[%s] session creation failed again (%v); retrying in %s", reason, err, backoff)
			}
			select {
			case <-time.After(backoff):
			case <-p.stopCh:
				return
			}
			backoff *= 2
			if backoff > poolCreateBackoffMax {
				backoff = poolCreateBackoffMax
			}
			continue
		}
		p.stock(id, reason)
		return
	}
}

// stock puts a freshly created session into the batch, or deletes it if
// shutdown raced in first (never stockpile sessions nobody will consume).
func (p *SessionPool) stock(id, reason string) {
	select {
	case p.ready <- id:
		p.log.Printf("[%s] session ready: %s (%d/%d)", reason, id, len(p.ready), p.size)
	case <-p.stopCh:
		p.deleteOne(id, "shutdown-race")
	}
}

// deleteOne deletes a single session upstream, best-effort.
func (p *SessionPool) deleteOne(id, reason string) {
	ctx, cancel := context.WithTimeout(context.Background(), poolOpTimeout)
	defer cancel()
	if err := p.backend.DeleteChatSession(ctx, id); err != nil {
		warnf("[pool:%s] warning: failed to delete session %s: %v", reason, id, err)
		return
	}
	debugf("[pool:%s] deleted chat session: %s", reason, id)
}
