package web

import (
	"compress/gzip"
	"net/http"
	"strings"
)

// compress gzips a response when the client asked for it and the body is
// the kind that shrinks.
//
// The globe lists all five hundred sightings in its markup, because that
// list is what the page is when JavaScript does not run. That makes it some
// 380KB of HTML, which is nothing on a desk and a long wait on the wifi of
// a conference hall. It gzips to about a fifteenth of that.
//
// Photographs and recordings are already compressed and are left alone:
// running them through gzip costs processor time to make them very slightly
// larger.
func compress(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Say the response varies whether or not this one is compressed, so
		// a cache in between never serves a gzipped body to a client that
		// cannot read it.
		w.Header().Add("Vary", "Accept-Encoding")

		if !acceptsGzip(r.Header.Get("Accept-Encoding")) {
			next.ServeHTTP(w, r)
			return
		}

		c := &compressor{ResponseWriter: w}
		defer c.close()

		next.ServeHTTP(c, r)
	})
}

// acceptsGzip reports whether the header offers gzip. A client may list it
// and then refuse it with a quality of zero, which is a way of saying it
// would rather not.
func acceptsGzip(header string) bool {
	for _, part := range strings.Split(header, ",") {
		fields := strings.Split(strings.TrimSpace(part), ";")
		if !strings.EqualFold(strings.TrimSpace(fields[0]), "gzip") {
			continue
		}
		for _, p := range fields[1:] {
			if strings.ReplaceAll(strings.TrimSpace(p), " ", "") == "q=0" {
				return false
			}
		}
		return true
	}
	return false
}

// compressible reports whether a media type is worth gzipping. Everything
// here is text; everything absent is either already compressed or too small
// to care about.
func compressible(contentType string) bool {
	media, _, _ := strings.Cut(contentType, ";")
	switch strings.ToLower(strings.TrimSpace(media)) {
	case "text/html", "text/css", "text/plain", "text/xml", "text/javascript",
		"application/json", "application/javascript", "application/xml",
		"image/svg+xml":
		return true
	default:
		return false
	}
}

// compressor is the ResponseWriter the handler below writes through. It
// decides once, at the moment the status is settled, whether this body is
// going to be compressed.
type compressor struct {
	http.ResponseWriter

	gz      *gzip.Writer
	decided bool
	sent    bool
}

// WriteHeader settles the question and then sends the header, which must
// happen in that order: the decision adds and removes header fields.
func (c *compressor) WriteHeader(status int) {
	if c.sent {
		return
	}
	c.decide(status)
	c.sent = true
	c.ResponseWriter.WriteHeader(status)
}

// Write sends the header first if the handler never did, which is what
// net/http would otherwise do on its behalf.
func (c *compressor) Write(b []byte) (int, error) {
	if !c.sent {
		// net/http sniffs an unset content type from the first bytes. Do it
		// here instead, because the decision needs the type before the
		// header goes out.
		if c.Header().Get("Content-Type") == "" {
			c.Header().Set("Content-Type", http.DetectContentType(b))
		}
		c.WriteHeader(http.StatusOK)
	}
	if c.gz != nil {
		return c.gz.Write(b)
	}
	return c.ResponseWriter.Write(b)
}

// Flush passes a flush through the gzip writer, so a handler that streams
// still reaches the client.
func (c *compressor) Flush() {
	if c.gz != nil {
		_ = c.gz.Flush()
	}
	if f, ok := c.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// decide works out whether this response is compressed, and if it is,
// corrects the header to say so.
func (c *compressor) decide(status int) {
	if c.decided {
		return
	}
	c.decided = true

	header := c.Header()
	switch {
	// Somebody else already encoded it.
	case header.Get("Content-Encoding") != "":
		return
	// No body to speak of.
	case status == http.StatusNoContent, status == http.StatusNotModified:
		return
	case !compressible(header.Get("Content-Type")):
		return
	}

	header.Set("Content-Encoding", "gzip")
	// The length was measured before compression, so it is now wrong. A
	// wrong Content-Length is worse than none: it truncates the body.
	header.Del("Content-Length")
	// Byte ranges are offsets into the uncompressed body and no longer mean
	// what they say.
	header.Del("Accept-Ranges")

	c.gz = gzip.NewWriter(c.ResponseWriter)
}

// close finishes the gzip stream. A handler that wrote nothing at all never
// started one.
func (c *compressor) close() {
	if c.gz != nil {
		_ = c.gz.Close()
	}
}
