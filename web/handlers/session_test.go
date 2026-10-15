package handlers_test

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"uuid"
	"workshop/internal/domain/game"
	"workshop/web/handlers"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// store writes g and returns the cookie the session set for it.
func store(t *testing.T, session handlers.Session, g game.Game) *http.Cookie {
	t.Helper()

	rec := httptest.NewRecorder()
	require.NoError(t, session.Write(rec, g))

	cookies := rec.Result().Cookies()
	require.Len(t, cookies, 1)

	return cookies[0]
}

// readBack asks the session for the game a request carrying value holds.
func readBack(t *testing.T, session handlers.Session, value string) (game.Game, error) {
	t.Helper()

	r := httptest.NewRequest(http.MethodGet, "/play", nil)
	r.AddCookie(&http.Cookie{Name: "safari", Value: value})

	return session.Read(r)
}

// repack rewrites a cookie's payload, leaving the signature it arrived
// with, which is what someone editing their score in devtools does.
func repack(t *testing.T, value, from, to string) string {
	t.Helper()

	payload, signature, ok := strings.Cut(value, ".")
	require.True(t, ok)

	body, err := base64.RawURLEncoding.DecodeString(payload)
	require.NoError(t, err)
	require.Contains(t, string(body), from)

	edited := strings.Replace(string(body), from, to, 1)

	return base64.RawURLEncoding.EncodeToString([]byte(edited)) + "." + signature
}

func TestSession(t *testing.T) {
	t.Parallel()

	session := handlers.NewSession()

	played := game.New([]uuid.UUID{uuid.New(), uuid.New(), uuid.New()})
	played.Record(game.Result{DistanceKm: 12.5, Points: 4200})

	t.Run("Round trips a game", func(t *testing.T) {
		t.Parallel()

		got, err := readBack(t, session, store(t, session, played).Value)
		require.NoError(t, err)
		assert.Equal(t, played, got)
	})

	t.Run("Is not readable by page scripts", func(t *testing.T) {
		t.Parallel()

		cookie := store(t, session, played)
		assert.True(t, cookie.HttpOnly)
		assert.Equal(t, http.SameSiteLaxMode, cookie.SameSite)
	})

	t.Run("A request with no cookie carries no game", func(t *testing.T) {
		t.Parallel()

		_, err := session.Read(httptest.NewRequest(http.MethodGet, "/play", nil))
		assert.Error(t, err)
	})

	t.Run("Rejects an edited score", func(t *testing.T) {
		t.Parallel()

		edited := repack(t, store(t, session, played).Value, `"points":4200`, `"points":5000`)

		_, err := readBack(t, session, edited)
		assert.Error(t, err, "a rewritten score must not verify")
	})

	t.Run("Rejects an edited signature", func(t *testing.T) {
		t.Parallel()

		payload, signature, ok := strings.Cut(store(t, session, played).Value, ".")
		require.True(t, ok)

		_, err := readBack(t, session, payload+"."+strings.Repeat("A", len(signature)))
		assert.Error(t, err)
	})

	t.Run("Rejects a game signed with another key", func(t *testing.T) {
		t.Parallel()

		elsewhere := handlers.NewSession()

		_, err := readBack(t, session, store(t, elsewhere, played).Value)
		assert.Error(t, err, "each run's key must only accept its own cookies")
	})

	t.Run("Rejects a cookie that isn't a payload and a signature", func(t *testing.T) {
		t.Parallel()

		for name, value := range map[string]string{
			"No separator": "notsignedatall",
			"Empty":        "",
			"Not base64":   "!!!.???",
		} {
			t.Run(name, func(t *testing.T) {
				_, err := readBack(t, session, value)
				assert.Error(t, err)
			})
		}
	})
}
