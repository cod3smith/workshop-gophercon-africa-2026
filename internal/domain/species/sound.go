package species

import (
	"context"
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
}

// soundAnswer is one remembered lookup.
type soundAnswer struct {
	recording Recording
	found     bool
	expires   time.Time
}

// soundFlight is a lookup under way. Its answer may be read once done is
// closed.
type soundFlight struct {
	done   chan struct{}
	answer soundAnswer
}

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
	flight := s.begin(ctx, scientificName)
	if flight == nil {
		return Recording{}, false
	}

	select {
	case <-flight.done:
		return flight.answer.recording, flight.answer.found
	case <-ctx.Done():
		return Recording{}, false
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

// fetch asks the source and keeps what it says.
func (s *Sounds) fetch(ctx context.Context, name string, flight *soundFlight) {
	ctx, cancel := context.WithTimeout(ctx, SoundLookupTimeout)
	defer cancel()

	answer, ttl := soundAnswer{}, SoundTTL

	recording, found, err := s.source.Sound(ctx, name)
	if err != nil {
		// A failure says nothing about whether a recording exists, so it is
		// not remembered as a day of silence; but it is not retried on
		// every view either, as a source that is struggling stays so.
		s.logger.WarnContext(ctx, "species: looking for a recording of "+name+": "+err.Error())
		ttl = SoundFailureTTL
	} else {
		answer = soundAnswer{recording: recording, found: found}
	}
	answer.expires = s.now().Add(ttl)

	s.mu.Lock()
	s.remember[name] = answer
	delete(s.flying, name)
	s.mu.Unlock()

	flight.answer = answer
	close(flight.done)
}
