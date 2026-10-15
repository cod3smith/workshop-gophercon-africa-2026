package web

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// serve runs the handler through compress and returns the recorded response.
func serve(t *testing.T, accept string, handler http.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	if accept != "" {
		r.Header.Set("Accept-Encoding", accept)
	}

	w := httptest.NewRecorder()
	compress(handler).ServeHTTP(w, r)

	return w
}

// sent is the header as a client would have received it: the copy net/http
// takes when the status line goes out. The recorder's own Header() is the
// live map the middleware is still holding, so a field written or deleted
// after the header has gone — too late to reach anybody — still shows up
// there, and a test reading it would call a truncating bug correct.
func sent(w *httptest.ResponseRecorder) http.Header {
	return w.Result().Header
}

// body returns the response body, ungzipping it when it says it is gzipped.
func body(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()

	if sent(w).Get("Content-Encoding") != "gzip" {
		return w.Body.String()
	}

	gz, err := gzip.NewReader(w.Body)
	require.NoError(t, err, "the body claims to be gzip and must be readable as gzip")

	out, err := io.ReadAll(gz)
	require.NoError(t, err)

	return string(out)
}

func html(n int) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, strings.Repeat("<li>a sighting</li>", n))
	}
}

func TestCompress_ShrinksHTMLAndStaysReadable(t *testing.T) {
	t.Parallel()

	want := strings.Repeat("<li>a sighting</li>", 500)
	w := serve(t, "gzip", html(500))

	// Taken before the body is read: unpacking it drains the recorder, and
	// a length measured afterwards is zero however little was compressed.
	compressed := w.Body.Len()

	assert.Equal(t, "gzip", sent(w).Get("Content-Encoding"))
	assert.Equal(t, want, body(t, w), "the body must survive the round trip unchanged")
	assert.Less(t, compressed, len(want)/5, "a page of repeated markup should shrink a long way")
}

func TestCompress_LeavesABodyAloneWhenTheClientCannotReadIt(t *testing.T) {
	t.Parallel()

	for _, accept := range []string{"", "deflate, br", "gzip;q=0", "identity"} {
		w := serve(t, accept, html(10))

		assert.Empty(t, sent(w).Get("Content-Encoding"), "Accept-Encoding %q does not offer gzip", accept)
		assert.Contains(t, w.Body.String(), "<li>a sighting</li>")
	}
}

func TestCompress_AlwaysSaysTheResponseVaries(t *testing.T) {
	t.Parallel()

	// Both ways: a cache must not hand a gzipped body to a client that
	// asked for plain text, and the header is how it knows.
	for _, accept := range []string{"gzip", ""} {
		w := serve(t, accept, html(10))
		assert.Contains(t, sent(w).Values("Vary"), "Accept-Encoding")
	}
}

func TestCompress_LeavesAlreadyCompressedBodiesAlone(t *testing.T) {
	t.Parallel()

	types := []string{"image/jpeg", "audio/mpeg", "audio/mp4", "image/png", "font/woff2"}
	for _, ct := range types {
		w := serve(t, "gzip", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", ct)
			_, _ = w.Write([]byte(strings.Repeat("x", 2048)))
		})

		assert.Empty(t, sent(w).Get("Content-Encoding"), "%s is already compressed", ct)
	}
}

func TestCompress_DoesNotReEncode(t *testing.T) {
	t.Parallel()

	w := serve(t, "gzip", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("Content-Encoding", "br")
		_, _ = io.WriteString(w, "already encoded")
	})

	assert.Equal(t, "br", sent(w).Get("Content-Encoding"))
	assert.Equal(t, "already encoded", w.Body.String())
}

func TestCompress_DropsTheLengthItJustInvalidated(t *testing.T) {
	t.Parallel()

	// A Content-Length measured before compression truncates the body if it
	// survives into a gzipped response.
	w := serve(t, "gzip", func(w http.ResponseWriter, _ *http.Request) {
		payload := strings.Repeat("<p>hello</p>", 200)
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		w.Header().Set("Accept-Ranges", "bytes")
		_, _ = io.WriteString(w, payload)
	})

	assert.Equal(t, "gzip", sent(w).Get("Content-Encoding"))
	assert.Empty(t, sent(w).Get("Content-Length"))
	assert.Empty(t, sent(w).Get("Accept-Ranges"), "ranges index the uncompressed body")
	assert.Equal(t, strings.Repeat("<p>hello</p>", 200), body(t, w))
}

func TestCompress_KeepsTheStatusTheHandlerChose(t *testing.T) {
	t.Parallel()

	w := serve(t, "gzip", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, "<p>no such sighting</p>")
	})

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, "<p>no such sighting</p>", body(t, w))
}

func TestCompress_LeavesABodilessResponseAlone(t *testing.T) {
	t.Parallel()

	for _, status := range []int{http.StatusNoContent, http.StatusNotModified} {
		w := serve(t, "gzip", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(status)
		})

		assert.Equal(t, status, w.Code)
		assert.Empty(t, sent(w).Get("Content-Encoding"))
		assert.Empty(t, w.Body.Bytes())
	}
}

func TestCompress_SniffsATypeTheHandlerNeverSet(t *testing.T) {
	t.Parallel()

	// net/http would sniff this itself; the middleware has to do it first,
	// because the decision needs a type before the header goes out.
	w := serve(t, "gzip", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "<!doctype html><title>a</title>"+strings.Repeat("<p>x</p>", 200))
	})

	assert.Equal(t, "gzip", sent(w).Get("Content-Encoding"))
	assert.Contains(t, body(t, w), "<!doctype html>")
}

func TestCompress_WritesNothingForAHandlerThatWroteNothing(t *testing.T) {
	t.Parallel()

	w := serve(t, "gzip", func(http.ResponseWriter, *http.Request) {})

	assert.Empty(t, w.Body.Bytes(), "an empty handler must not produce a gzip header and trailer")
	assert.Empty(t, sent(w).Get("Content-Encoding"))
}

func TestCompress_FlushPushesWhatIsBufferedToTheClient(t *testing.T) {
	t.Parallel()

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()

	var buffered, flushed int
	compress(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		rw.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(rw, strings.Repeat("<p>a sighting</p>", 200))
		buffered = w.Body.Len()

		// Asked for the way a handler asks: through net/http's own
		// accessor, which must find the wrapper's Flush and not reach past
		// it to the writer underneath, where the gzip buffer is not.
		require.NoError(t, http.NewResponseController(rw).Flush())
		flushed = w.Body.Len()
	})).ServeHTTP(w, r)

	// Only the gzip header has gone out; the three kilobytes of markup are
	// still inside the writer's window, where a reader cannot reach them.
	assert.Equal(t, 10, buffered, "the compressed bytes wait until something empties the writer")
	assert.Greater(t, flushed, buffered, "a flush must put those bytes on the wire")
	assert.Equal(t, strings.Repeat("<p>a sighting</p>", 200), body(t, w))
}
