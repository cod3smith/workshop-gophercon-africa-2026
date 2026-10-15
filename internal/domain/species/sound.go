package species

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// Recording is a sound of an animal that a player can hear: where the file
// is, what it is, and the credit its licence asks to be shown beside it.
type Recording struct {
	// URL is the playable file, kept exactly as the source gave it.
	URL string
	// ContentType is the media type of the file, such as "audio/mp4".
	ContentType string
	// Licence is the Creative Commons code of the recording itself, such as
	// "cc-by-nc". It is not the licence of the observation it came with.
	Licence string
	// Attribution is the credit line the licence requires, as the source
	// formatted it: who made the recording and under which terms.
	Attribution string
}

// SoundSource is where recordings come from. The iNaturalist client
// satisfies it: it reports false for a species with no recording, and an
// error when it could not find out.
type SoundSource interface {
	Sound(ctx context.Context, scientificName string) (Recording, bool, error)
}

const (
	// SoundTTL is how long an answer from the source is trusted, whether it
	// found a recording or found none. Recordings are added rarely, and the
	// source is rate limited, so a day is no hardship.
	SoundTTL = 24 * time.Hour
	// SoundFailureTTL is how long a failed lookup is remembered. It says
	// nothing about whether a recording exists, so it is kept brief, but
	// long enough that a source that is struggling is not asked again on
	// every reveal.
	SoundFailureTTL = time.Minute
	// SoundLookupTimeout is how long the source is given to answer. A lookup
	// outlives the page that asked for it, because the answer is kept for
	// the next one, so this is not the page's deadline. iNaturalist takes
	// several seconds, which is far longer than a reveal can wait.
	SoundLookupTimeout = 20 * time.Second
	// SoundLookups is how many lookups may be out at once. The source is
	// rate limited and a room of players each reveals a different animal,
	// so the rest wait their turn rather than going out together.
	SoundLookups = 2
)

// Sounds finds the recording of a species, if there is one, and remembers
// the answer so a page view is never a round trip.
//
// Absence is remembered as firmly as presence: most species have no
// recording, and forgetting that would send them to the source on every
// reveal. The memory lives in the process, which is as long as it needs to
// last for a workshop.
//
// A lookup is slow and a page is impatient, so a lookup does not belong to
// the page that started it. It runs on its own, the page waits only as long
// as it can afford to, and whatever the lookup finds is kept for the next
// page. Players asking about a species already being looked up share that
// one lookup.
type Sounds struct {
	logger *slog.Logger
	source SoundSource
	now    func() time.Time

	mu       sync.Mutex
	remember map[string]soundAnswer
	flying   map[string]*soundFlight

	// permits is a semaphore over the lookups in flight. It is taken by the
	// lookup, on its own goroutine, and never by a caller: callers are on
	// the request path and must not wait for it.
	permits chan struct{}
}

// soundAnswer is one remembered lookup.
type soundAnswer struct {
	recording Recording
	// found says there is a recording. It is only meaningful when known.
	found bool
	// known says the source answered, whether or not it had a recording.
	// A lookup that failed is not known: it establishes nothing about
	// whether a recording exists, and a page must not say that it does.
	known   bool
	expires time.Time
}

// soundFlight is a lookup under way. Its answer may be read once done is
// closed.
type soundFlight struct {
	done   chan struct{}
	answer soundAnswer
}

// SoundOutcome says what a lookup established about a species.
//
// The distinction matters to a page. "There is no recording of this animal"
// and "nobody has asked yet" are different statements, and only the first is
// safe to put in front of a reader: a lookup takes several seconds against
// somebody else's server, so a page that treats an unfinished lookup as an
// absence tells its reader something false, and tells it most often on the
// first visit.
type SoundOutcome int

const (
	// SoundUnknown means the lookup has not finished, failed, or was never
	// possible. Nothing may be claimed either way.
	SoundUnknown SoundOutcome = iota
	// SoundNone means the source answered and has no recording.
	SoundNone
	// SoundFound means there is a recording to offer.
	SoundFound
)

// SoundOption configures a Sounds.
type SoundOption func(*Sounds)

// WithSoundClock sets where Sounds reads the time from, so a test can
// move it.
func WithSoundClock(now func() time.Time) SoundOption {
	return func(s *Sounds) { s.now = now }
}

// NewSounds returns Sounds asking source for what it does not remember.
// A nil source means no species has a recording.
func NewSounds(logger *slog.Logger, source SoundSource, opts ...SoundOption) *Sounds {
	s := &Sounds{
		logger:   logger,
		source:   source,
		now:      time.Now,
		remember: map[string]soundAnswer{},
		flying:   map[string]*soundFlight{},
		permits:  make(chan struct{}, SoundLookups),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Sound returns the recording of the species, and false when there is none
// to offer. That is also the answer when the lookup failed, or is still
// running when ctx runs out: a page that offers a sound must read the same
// as one that has none, so a failure is logged here and goes no further.
func (s *Sounds) Sound(ctx context.Context, scientificName string) (Recording, bool) {
	recording, outcome := s.Lookup(ctx, scientificName)
	return recording, outcome == SoundFound
}

// Lookup is Sound, but it also says whether the answer is established.
// A caller that wants to write "no recording" needs to know the difference
// between a source that has none and a lookup that has not come back.
func (s *Sounds) Lookup(ctx context.Context, scientificName string) (Recording, SoundOutcome) {
	flight := s.begin(ctx, scientificName)
	if flight == nil {
		return Recording{}, SoundUnknown
	}

	select {
	case <-flight.done:
		return flight.answer.recording, flight.answer.outcome()
	case <-ctx.Done():
		// Still running. It will finish and be kept for the next caller,
		// but right now nothing is known.
		return Recording{}, SoundUnknown
	}
}

// outcome reads the answer as one of the three things it can be.
func (a soundAnswer) outcome() SoundOutcome {
	switch {
	case !a.known:
		return SoundUnknown
	case a.found:
		return SoundFound
	default:
		return SoundNone
	}
}

// Warm starts finding the recording of the species without waiting for it,
// so the answer is ready when the reveal asks. It does nothing for a
// species already remembered or already being looked up.
func (s *Sounds) Warm(ctx context.Context, scientificName string) {
	s.begin(ctx, scientificName)
}

// begin returns the lookup for the species: one already done and
// remembered, one under way, or a new one it starts. It is nil when there
// is nothing to look up.
func (s *Sounds) begin(ctx context.Context, name string) *soundFlight {
	if s == nil || s.source == nil || name == "" {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if answer, ok := s.remember[name]; ok {
		if s.now().Before(answer.expires) {
			done := &soundFlight{done: make(chan struct{}), answer: answer}
			close(done.done)
			return done
		}
		delete(s.remember, name)
	}

	if flight, ok := s.flying[name]; ok {
		return flight
	}

	flight := &soundFlight{done: make(chan struct{})}
	s.flying[name] = flight

	// The lookup outlives the page that started it, but keeps what the
	// page's context carries, such as its logger.
	go s.fetch(context.WithoutCancel(ctx), name, flight)

	return flight
}

// fetch asks the source and keeps what it says. However it ends, the
// lookup is settled: the species stops being under way and whoever is
// waiting on it is released.
//
// It runs on a goroutine of its own, out of reach of the server's
// per-request recovery, so a panic is caught here: left alone it would end
// the process and every player's game with it.
func (s *Sounds) fetch(ctx context.Context, name string, flight *soundFlight) {
	answer, ttl := soundAnswer{}, SoundFailureTTL

	defer func() {
		if r := recover(); r != nil {
			s.warn(ctx, fmt.Sprintf("species: looking for a recording of %s panicked: %v", name, r))
			answer, ttl = soundAnswer{}, SoundFailureTTL
		}
		s.settle(name, flight, answer, ttl)
	}()

	// Wait for a permit before the clock on the lookup starts, so queueing
	// does not eat the time the source is given. The lookup is already
	// registered as flying, so anyone asking meanwhile joins it.
	s.permits <- struct{}{}
	defer func() { <-s.permits }()

	ctx, cancel := context.WithTimeout(ctx, SoundLookupTimeout)
	defer cancel()

	recording, found, err := s.source.Sound(ctx, name)
	if err != nil {
		// A failure says nothing about whether a recording exists, so it is
		// not remembered as a day of silence; but it is not retried on
		// every view either, as a source that is struggling stays so.
		s.warn(ctx, "species: looking for a recording of "+name+": "+err.Error())
		return
	}

	answer, ttl = soundAnswer{recording: recording, found: found, known: true}, SoundTTL
}

// settle keeps the answer, ends the lookup and releases those waiting on it.
func (s *Sounds) settle(name string, flight *soundFlight, answer soundAnswer, ttl time.Duration) {
	s.mu.Lock()
	answer.expires = s.now().Add(ttl)
	s.remember[name] = answer
	delete(s.flying, name)
	s.mu.Unlock()

	flight.answer = answer
	close(flight.done)
}

func (s *Sounds) warn(ctx context.Context, msg string) {
	if s.logger != nil {
		s.logger.WarnContext(ctx, msg)
	}
}
