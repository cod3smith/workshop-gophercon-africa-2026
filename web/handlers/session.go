package handlers

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"workshop/internal/domain/game"
)

// gameCookie is the cookie a game in progress travels in.
const gameCookie = "safari"

// Session keeps a game between requests, in a cookie the browser can read
// but not rewrite: the game travels as JSON with a signature beside it, and
// an edited score fails the signature and starts a new game.
//
// Holding it client-side means the server keeps nothing per player, so the
// game survives a restart of the handler and never leaks a session.
type Session struct {
	key []byte
}

// NewSession returns a Session signing with a key generated for this run of
// the program. Games don't outlive a restart, which for a game of five
// rounds is a trade worth making to avoid configuring a secret.
func NewSession() Session {
	key := make([]byte, 32)

	// crypto/rand.Read never reports an error: it crashes the program if
	// the operating system can't give it randomness, which is the right
	// answer for a key we're about to sign with.
	_, _ = rand.Read(key)

	return Session{key: key}
}

// errNoGame is returned when the request carries no game, or carries one
// that has been tampered with. Either way the caller starts a new one.
var errNoGame = errors.New("handlers: no game in this request")

// Read returns the game the request carries.
func (s Session) Read(r *http.Request) (game.Game, error) {
	cookie, err := r.Cookie(gameCookie)
	if err != nil {
		return game.Game{}, errNoGame
	}

	payload, signature, ok := strings.Cut(cookie.Value, ".")
	if !ok {
		return game.Game{}, errNoGame
	}

	body, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return game.Game{}, errNoGame
	}

	got, err := base64.RawURLEncoding.DecodeString(signature)
	if err != nil {
		return game.Game{}, errNoGame
	}

	// Compared in constant time, so the signature can't be guessed a byte
	// at a time by timing the responses.
	if !hmac.Equal(got, s.sign(body)) {
		return game.Game{}, errNoGame
	}

	var g game.Game
	if err = json.Unmarshal(body, &g); err != nil {
		return game.Game{}, errNoGame
	}

	return g, nil
}

// Write stores the game on the response.
func (s Session) Write(w http.ResponseWriter, g game.Game) error {
	body, err := json.Marshal(g)
	if err != nil {
		return err
	}

	value := base64.RawURLEncoding.EncodeToString(body) + "." +
		base64.RawURLEncoding.EncodeToString(s.sign(body))

	http.SetCookie(w, &http.Cookie{
		Name:     gameCookie,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	return nil
}

// sign returns the signature for a cookie's body.
func (s Session) sign(body []byte) []byte {
	mac := hmac.New(sha256.New, s.key)
	mac.Write(body)
	return mac.Sum(nil)
}
