package species_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"maps"
	"strconv"
	"sync"
	"testing"
	"time"
	"workshop/internal/common/logs"
	"workshop/internal/domain/species"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func quietLogger() *slog.Logger { return logs.NewProduction(io.Discard, slog.LevelError) }

// soundsStub is a recording source that counts the questions put to it.
type soundsStub struct {
	mu        sync.Mutex
	calls     map[string]int
	recording species.Recording
	found     bool
	err       error
}

func (s *soundsStub) Sound(_ context.Context, name string) (species.Recording, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.calls == nil {
		s.calls = map[string]int{}
	}
	s.calls[name]++

	return s.recording, s.found, s.err
}

func (s *soundsStub) asked(name string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls[name]
}

var lionRoar = species.Recording{
	URL:         "https://static.inaturalist.org/sounds/1.m4a?1",
	ContentType: "audio/mp4",
	Licence:     "cc-by-nc",
	Attribution: "(c) A Recordist, some rights reserved (CC BY-NC)",
}

// clock is a time that only moves when a test says so.
type clock struct{ now time.Time }

func (c *clock) Now() time.Time          { return c.now }
func (c *clock) Advance(d time.Duration) { c.now = c.now.Add(d) }

func newSounds(source species.SoundSource) (*species.Sounds, *clock) {
	c := &clock{now: time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC)}
	return species.NewSounds(quietLogger(), source, species.WithSoundClock(c.Now)), c
}

func TestSounds_RemembersARecording(t *testing.T) {
	t.Parallel()

	stub := &soundsStub{recording: lionRoar, found: true}
	sounds, clk := newSounds(stub)

	for range 3 {
		got, ok := sounds.Sound(t.Context(), "Panthera leo")
		require.True(t, ok)
		assert.Equal(t, lionRoar, got)
	}
	assert.Equal(t, 1, stub.asked("Panthera leo"), "a page view is not a round trip")

	clk.Advance(species.SoundTTL + time.Second)
	_, ok := sounds.Sound(t.Context(), "Panthera leo")
	require.True(t, ok)
	assert.Equal(t, 2, stub.asked("Panthera leo"), "an old answer is asked for again")
}

// Most species have no recording, so if only hits were remembered the common
// case would go to iNaturalist on every reveal.
func TestSounds_RemembersAbsenceToo(t *testing.T) {
	t.Parallel()

	stub := &soundsStub{found: false}
	sounds, clk := newSounds(stub)

	for range 5 {
		_, ok := sounds.Sound(t.Context(), "Acinonyx jubatus")
		assert.False(t, ok)
	}
	assert.Equal(t, 1, stub.asked("Acinonyx jubatus"))

	clk.Advance(species.SoundTTL + time.Second)
	sounds.Sound(t.Context(), "Acinonyx jubatus")
	assert.Equal(t, 2, stub.asked("Acinonyx jubatus"))
}

func TestSounds_KeepsSpeciesApart(t *testing.T) {
	t.Parallel()

	stub := &soundsStub{found: false}
	sounds, _ := newSounds(stub)

	sounds.Sound(t.Context(), "Acinonyx jubatus")
	sounds.Sound(t.Context(), "Panthera leo")
	sounds.Sound(t.Context(), "Panthera leo")

	assert.Equal(t, 1, stub.asked("Acinonyx jubatus"))
	assert.Equal(t, 1, stub.asked("Panthera leo"))
}

// A failed lookup says nothing about whether a recording exists, so it is
// not remembered as a day of silence, but it is not retried on every view
// either: a rate-limited service asked again at once stays rate-limited.
func TestSounds_AFailureIsNoRecordingForAShortWhile(t *testing.T) {
	t.Parallel()

	stub := &soundsStub{err: errors.New("429 too many requests")}
	sounds, clk := newSounds(stub)

	for range 3 {
		got, ok := sounds.Sound(t.Context(), "Panthera leo")
		assert.False(t, ok)
		assert.Zero(t, got)
	}
	assert.Equal(t, 1, stub.asked("Panthera leo"))

	clk.Advance(species.SoundFailureTTL + time.Second)
	stub.mu.Lock()
	stub.err, stub.recording, stub.found = nil, lionRoar, true
	stub.mu.Unlock()

	got, ok := sounds.Sound(t.Context(), "Panthera leo")
	require.True(t, ok, "once the pause is over, the species is asked about again")
	assert.Equal(t, lionRoar, got)
}

func TestSounds_NeedsAName(t *testing.T) {
	t.Parallel()

	stub := &soundsStub{recording: lionRoar, found: true}
	sounds, _ := newSounds(stub)

	_, ok := sounds.Sound(t.Context(), "")
	assert.False(t, ok)
	assert.Zero(t, stub.asked(""))
}

func TestSounds_NoSourceIsNoRecording(t *testing.T) {
	t.Parallel()

	sounds := species.NewSounds(quietLogger(), nil)

	_, ok := sounds.Sound(t.Context(), "Panthera leo")
	assert.False(t, ok)
}

func TestSounds_IsSafeToShare(t *testing.T) {
	t.Parallel()

	stub := &soundsStub{recording: lionRoar, found: true}
	sounds, _ := newSounds(stub)

	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() { sounds.Sound(t.Context(), "Panthera leo") })
	}
	wg.Wait()
}

// gate is a source that answers only when a test lets it, so a lookup can
// be slower than the page waiting on it.
type gate struct {
	release   chan struct{}
	started   chan struct{}
	mu        sync.Mutex
	calls     int
	recording species.Recording
}

func newGate() *gate {
	return &gate{release: make(chan struct{}), started: make(chan struct{}, 10), recording: lionRoar}
}

func (g *gate) Sound(ctx context.Context, _ string) (species.Recording, bool, error) {
	g.mu.Lock()
	g.calls++
	g.mu.Unlock()
	g.started <- struct{}{}

	select {
	case <-g.release:
		return g.recording, true, nil
	case <-ctx.Done():
		return species.Recording{}, false, ctx.Err()
	}
}

func (g *gate) asked() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.calls
}

// The source can take longer than a reveal can wait (iNaturalist takes
// several seconds), but the answer it eventually gives is still kept, so the
// next reveal has it.
func TestSounds_ASlowLookupFinishesAfterThePageHasGivenUp(t *testing.T) {
	t.Parallel()

	g := newGate()
	sounds := species.NewSounds(quietLogger(), g)

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	_, ok := sounds.Sound(ctx, "Panthera leo")
	assert.False(t, ok, "the page does not wait for a slow source")

	close(g.release)
	require.Eventually(t, func() bool {
		_, ok := sounds.Sound(t.Context(), "Panthera leo")
		return ok
	}, 2*time.Second, 5*time.Millisecond, "the lookup the page gave up on still lands")

	assert.Equal(t, 1, g.asked(), "and was not started again")
}

// Warming is how a reveal has its answer ready: the question is on screen
// for a while before the guess, and the lookup runs in that time.
func TestSounds_WarmingHasTheAnswerReadyForTheReveal(t *testing.T) {
	t.Parallel()

	g := newGate()
	sounds := species.NewSounds(quietLogger(), g)

	sounds.Warm(t.Context(), "Panthera leo")
	<-g.started
	close(g.release)

	require.Eventually(t, func() bool {
		_, ok := sounds.Sound(t.Context(), "Panthera leo")
		return ok
	}, 2*time.Second, 5*time.Millisecond)
	assert.Equal(t, 1, g.asked())
}

func TestSounds_WarmingDoesNotWait(t *testing.T) {
	t.Parallel()

	g := newGate()
	sounds := species.NewSounds(quietLogger(), g)

	done := make(chan struct{})
	go func() {
		sounds.Warm(t.Context(), "Panthera leo")
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Warm waited for the source")
	}
	close(g.release)
}

func TestSounds_SharesOneLookupBetweenPlayers(t *testing.T) {
	t.Parallel()

	g := newGate()
	sounds := species.NewSounds(quietLogger(), g)

	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() { sounds.Sound(t.Context(), "Panthera leo") })
	}
	<-g.started
	close(g.release)
	wg.Wait()

	assert.Equal(t, 1, g.asked(), "ten players at once are one question to a rate-limited service")
}

func TestSounds_WarmingTheWarmIsFree(t *testing.T) {
	t.Parallel()

	stub := &soundsStub{found: false}
	sounds, _ := newSounds(stub)

	sounds.Sound(t.Context(), "Acinonyx jubatus")
	sounds.Warm(t.Context(), "Acinonyx jubatus")
	sounds.Warm(t.Context(), "")

	assert.Equal(t, 1, stub.asked("Acinonyx jubatus"))
}

// counting is a source that holds every lookup until released and records
// how many are under way at once.
type counting struct {
	release chan struct{}
	started chan string

	mu      sync.Mutex
	running int
	peak    int
	calls   map[string]int
}

func newCounting() *counting {
	return &counting{release: make(chan struct{}), started: make(chan string, 32), calls: map[string]int{}}
}

func (c *counting) Sound(ctx context.Context, name string) (species.Recording, bool, error) {
	c.mu.Lock()
	c.running++
	c.peak = max(c.peak, c.running)
	c.calls[name]++
	c.mu.Unlock()
	c.started <- name

	defer func() {
		c.mu.Lock()
		c.running--
		c.mu.Unlock()
	}()

	select {
	case <-c.release:
		return lionRoar, true, nil
	case <-ctx.Done():
		return species.Recording{}, false, ctx.Err()
	}
}

func (c *counting) stats() (peak int, calls map[string]int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.peak, maps.Clone(c.calls)
}

// Twenty-five players on one network, each with a different animal, are
// twenty-five lookups; the service they ask is rate limited, so only a few
// are let out at once and the rest wait their turn.
func TestSounds_OnlyAFewLookupsRunAtOnce(t *testing.T) {
	t.Parallel()

	src := newCounting()
	sounds := species.NewSounds(quietLogger(), src)

	names := make([]string, 12)
	for i := range names {
		names[i] = "Species " + strconv.Itoa(i)
	}

	begun := make(chan struct{})
	go func() {
		defer close(begun)
		for _, name := range names {
			sounds.Warm(t.Context(), name)
		}
	}()
	select {
	case <-begun:
	case <-time.After(2 * time.Second):
		t.Fatal("Warm waited for a permit; it runs on the request path and must not block")
	}

	for range species.SoundLookups {
		<-src.started
	}
	time.Sleep(50 * time.Millisecond)
	got, _ := src.stats()
	assert.Equal(t, species.SoundLookups, got, "no more than the permits are out at once")

	close(src.release)
	for _, name := range names {
		require.Eventually(t, func() bool {
			_, ok := sounds.Sound(t.Context(), name)
			return ok
		}, 5*time.Second, 5*time.Millisecond, name+" is looked up in the end")
	}

	got, calls := src.stats()
	assert.LessOrEqual(t, got, species.SoundLookups)
	for _, name := range names {
		assert.Equal(t, 1, calls[name], name+" is looked up once")
	}
}

// A lookup waiting for its turn is still a lookup under way: callers for
// the same species join it rather than queueing a second, and the wait does
// not come out of the time the source is given.
func TestSounds_AQueuedLookupIsStillSharedAndKeepsItsTimeBudget(t *testing.T) {
	t.Parallel()

	src := newCounting()
	sounds := species.NewSounds(quietLogger(), src)

	for i := range species.SoundLookups {
		sounds.Warm(t.Context(), "Busy "+strconv.Itoa(i))
	}
	for range species.SoundLookups {
		<-src.started
	}

	// Every permit is held, so this one queues.
	sounds.Warm(t.Context(), "Queued")
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Millisecond)
	defer cancel()
	_, ok := sounds.Sound(ctx, "Queued")
	assert.False(t, ok, "a page that cannot wait gets no audio")

	close(src.release)
	require.Eventually(t, func() bool {
		_, ok := sounds.Sound(t.Context(), "Queued")
		return ok
	}, 5*time.Second, 5*time.Millisecond)

	_, calls := src.stats()
	assert.Equal(t, 1, calls["Queued"], "everyone who asked about it shared one lookup")
}

// panicking is a source that blows up.
type panicking struct {
	mu    sync.Mutex
	calls int
	then  species.SoundSource
}

func (p *panicking) Sound(ctx context.Context, name string) (species.Recording, bool, error) {
	p.mu.Lock()
	p.calls++
	n := p.calls
	p.mu.Unlock()

	if n == 1 || p.then == nil {
		panic("the source blew up")
	}
	return p.then.Sound(ctx, name)
}

// A lookup runs on its own goroutine, where the server's per-request
// recovery cannot see it, so a panic there would end the process and every
// player's game with it.
func TestSounds_ASourceThatPanicsIsAFailedLookup(t *testing.T) {
	t.Parallel()

	src := &panicking{then: &soundsStub{recording: lionRoar, found: true}}
	sounds, clk := newSounds(src)

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()

	start := time.Now()
	got, ok := sounds.Sound(ctx, "Panthera leo")
	assert.False(t, ok)
	assert.Zero(t, got)
	assert.Less(t, time.Since(start), time.Second, "the caller is told, not left waiting for a lookup that died")

	// The species is not wedged: after the brief pause it is asked about again.
	clk.Advance(species.SoundFailureTTL + time.Second)
	got, ok = sounds.Sound(ctx, "Panthera leo")
	require.True(t, ok)
	assert.Equal(t, lionRoar, got)
}

func TestSounds_APanicGivesBackItsPermit(t *testing.T) {
	t.Parallel()

	src := &panicking{}
	sounds, clk := newSounds(src)

	// More panics than there are permits: if one were kept, the later
	// lookups would wait for ever.
	for i := range species.SoundLookups + 2 {
		clk.Advance(species.SoundFailureTTL + time.Second)
		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
		start := time.Now()
		_, ok := sounds.Sound(ctx, "Panthera leo")
		cancel()
		assert.False(t, ok)
		assert.Less(t, time.Since(start), time.Second, "lookup %d did not wait for a permit", i)
	}
}
