// Package inaturalist is the iNaturalist API client. The sightings'
// photographs already come from iNaturalist; this is where the sounds come
// from, since GBIF records none of its own.
package inaturalist

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"workshop/internal/common/httputil"
	"workshop/internal/domain/species"
)

// Client is the iNaturalist API client.
type Client struct {
	http *httputil.Client
}

// BaseURL is the URL of the iNaturalist API.
const BaseURL = "https://api.inaturalist.org/v1"

// New creates a new iNaturalist API client. An empty baseURL falls back to
// BaseURL; tests pass the address of a stub server.
func New(baseURL string) *Client {
	if baseURL == "" {
		baseURL = BaseURL
	}
	return &Client{
		http: httputil.New(httputil.Config{BaseURL: baseURL}),
	}
}

// observationsPerPage is how many observations with a sound are looked
// through for one the player may hear. Plenty are all-rights-reserved or
// in a format a browser cannot play, so the first is not always usable.
const observationsPerPage = 30

// Sound finds a recording of the species to play.
//
// It reports false, with no error, for a species iNaturalist does not
// know or has no usable recording of; that is the common case, not a
// failure. An error means the answer could not be had at all (the network,
// a rate limit, a body that is not iNaturalist's), so a caller can tell
// "nothing to hear" from "could not ask" and cache them differently.
func (c *Client) Sound(ctx context.Context, scientificName string) (species.Recording, bool, error) {
	scientificName = strings.TrimSpace(scientificName)
	if scientificName == "" {
		return species.Recording{}, false, nil
	}

	id, ok, err := c.taxon(ctx, scientificName)
	if err != nil || !ok {
		return species.Recording{}, false, err
	}

	result, err := c.http.Do[observationsResponse](ctx, httputil.Request{
		Method: http.MethodGet,
		Path:   "/observations",
		Query: url.Values{
			"taxon_id": {strconv.Itoa(id)},
			"sounds":   {"true"},
			"per_page": {strconv.Itoa(observationsPerPage)},
		},
	})
	if err != nil {
		return species.Recording{}, false, fmt.Errorf("inaturalist: observations of taxon %d: %w", id, err)
	}

	for _, observation := range result.Results {
		for _, sound := range observation.Sounds {
			if recording, ok := sound.recording(); ok {
				return recording, true, nil
			}
		}
	}

	return species.Recording{}, false, nil
}

// taxon resolves a scientific name to the iNaturalist taxon of exactly that
// species. The search is fuzzy (a lion also matches the cave lion), so the
// hit has to carry the name that was asked for.
func (c *Client) taxon(ctx context.Context, scientificName string) (int, bool, error) {
	result, err := c.http.Do[taxaResponse](ctx, httputil.Request{
		Method: http.MethodGet,
		Path:   "/taxa",
		Query:  url.Values{"q": {scientificName}, "rank": {"species"}},
	})
	if err != nil {
		return 0, false, fmt.Errorf("inaturalist: finding taxon %q: %w", scientificName, err)
	}

	for _, t := range result.Results {
		if strings.EqualFold(t.Name, scientificName) {
			return t.ID, true, nil
		}
	}

	return 0, false, nil
}

// playable are the media types a browser's audio element plays. A file of
// any other type would fail silently in the player, so it is skipped.
var playable = map[string]bool{
	"audio/mpeg":  true,
	"audio/mp4":   true,
	"audio/aac":   true,
	"audio/x-wav": true,
	"audio/wav":   true,
	"audio/ogg":   true,
	"audio/webm":  true,
	"audio/flac":  true,
}

// recording turns a sound into a recording to play, and false when it
// cannot be offered: hidden, not licensed for sharing, nothing to credit it
// to, or not a file a browser plays.
func (s sound) recording() (species.Recording, bool) {
	if s.Hidden {
		return species.Recording{}, false
	}

	// A sound with no licence of its own is all rights reserved, whatever
	// the observation it came with says.
	if strings.TrimSpace(s.LicenseCode) == "" || strings.TrimSpace(s.Attribution) == "" {
		return species.Recording{}, false
	}

	contentType, _, _ := strings.Cut(strings.ToLower(s.FileContentType), ";")
	contentType = strings.TrimSpace(contentType)
	if !playable[contentType] {
		return species.Recording{}, false
	}

	// The address goes into a page, so only a web address will do.
	u, err := url.Parse(s.FileURL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return species.Recording{}, false
	}

	return species.Recording{
		URL:         s.FileURL,
		ContentType: contentType,
		Licence:     s.LicenseCode,
		Attribution: s.Attribution,
	}, true
}

// The raw responses, cut down to what the client reads.
//
// See: https://api.inaturalist.org/v1/docs/
type (
	taxaResponse struct {
		Results []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"results"`
	}
	observationsResponse struct {
		Results []struct {
			Sounds []sound `json:"sounds"`
		} `json:"results"`
	}
	// sound is one recording on an observation. Its licence is its own:
	// the observation carries a different one, which says nothing about
	// the recording.
	sound struct {
		FileURL         string `json:"file_url"`
		FileContentType string `json:"file_content_type"`
		LicenseCode     string `json:"license_code"`
		Attribution     string `json:"attribution"`
		Hidden          bool   `json:"hidden"`
	}
)
