package inaturalist

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// api stands in for iNaturalist: each path serves a fixture with a status,
// and the queries it was asked are kept so a test can see what was sent.
type api struct {
	taxaStatus, observationsStatus int
	taxa, observations             string

	mu      sync.Mutex
	queries map[string]url.Values
	agents  []string
}

func fixture(t *testing.T, name string) string {
	t.Helper()

	b, err := os.ReadFile("testdata/" + name)
	require.NoError(t, err)
	return string(b)
}

func (a *api) serve(t *testing.T) *Client {
	t.Helper()

	a.queries = map[string]url.Values{}
	respond := func(path string, status int, body string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			a.mu.Lock()
			a.queries[path] = r.URL.Query()
			a.agents = append(a.agents, r.UserAgent())
			a.mu.Unlock()

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(cmpStatus(status))
			_, _ = w.Write([]byte(body))
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/taxa", respond("taxa", a.taxaStatus, a.taxa))
	mux.HandleFunc("GET /v1/observations", respond("observations", a.observationsStatus, a.observations))

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return New(srv.URL + "/v1")
}

func cmpStatus(status int) int {
	if status == 0 {
		return http.StatusOK
	}
	return status
}

func TestClient_Sound(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		api        func(t *testing.T) *api
		wantOK     bool
		wantErr    bool
		wantURL    string
		wantType   string
		wantLic    string
		wantCredit string
	}{
		"a taxon with sounds gives the first one it may share": {
			api: func(t *testing.T) *api {
				return &api{taxa: fixture(t, "taxa_lion.json"), observations: fixture(t, "observations_lion_sounds.json")}
			},
			wantOK:     true,
			wantURL:    "https://static.inaturalist.org/sounds/2160774.wav?1789563327",
			wantType:   "audio/x-wav",
			wantLic:    "cc-by-nc",
			wantCredit: "(c) Africa Gomez, some rights reserved (CC BY-NC)",
		},
		"a taxon with no sounds is no recording": {
			api: func(t *testing.T) *api {
				return &api{taxa: fixture(t, "taxa_cheetah.json"), observations: fixture(t, "observations_none.json")}
			},
		},
		"sounds that cannot be shared or played are no recording": {
			api: func(t *testing.T) *api {
				return &api{taxa: fixture(t, "taxa_lion.json"), observations: fixture(t, "observations_unplayable.json")}
			},
		},
		"a name iNaturalist does not know is no recording": {
			api: func(t *testing.T) *api {
				return &api{taxa: fixture(t, "taxa_none.json")}
			},
		},
		"a search that only found a different species is no recording": {
			api: func(t *testing.T) *api {
				return &api{taxa: fixture(t, "taxa_cheetah.json")}
			},
		},
		"a rate limit is no recording": {
			api: func(t *testing.T) *api {
				return &api{taxaStatus: http.StatusTooManyRequests, taxa: `{"error":"slow down"}`}
			},
			wantErr: true,
		},
		"a server error on the observations is no recording": {
			api: func(t *testing.T) *api {
				return &api{taxa: fixture(t, "taxa_lion.json"), observationsStatus: http.StatusInternalServerError, observations: "boom"}
			},
			wantErr: true,
		},
		"a malformed taxa body is no recording": {
			api: func(t *testing.T) *api {
				return &api{taxa: `{"results": [`}
			},
			wantErr: true,
		},
		"a malformed observations body is no recording": {
			api: func(t *testing.T) *api {
				return &api{taxa: fixture(t, "taxa_lion.json"), observations: `<html>not json</html>`}
			},
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			a := tc.api(t)
			client := a.serve(t)

			rec, ok, err := client.Sound(context.Background(), "Panthera leo")

			assert.Equal(t, tc.wantOK, ok)
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			if !tc.wantOK {
				assert.Zero(t, rec, "no recording means an empty one, never half of one")
				return
			}

			assert.Equal(t, tc.wantURL, rec.URL)
			assert.Equal(t, tc.wantType, rec.ContentType)
			assert.Equal(t, tc.wantLic, rec.Licence)
			assert.Equal(t, tc.wantCredit, rec.Attribution)
		})
	}
}

func TestClient_Sound_AsksForTheSpeciesAndItsSounds(t *testing.T) {
	t.Parallel()

	a := &api{taxa: fixture(t, "taxa_lion.json"), observations: fixture(t, "observations_lion_sounds.json")}
	client := a.serve(t)

	_, ok, err := client.Sound(context.Background(), "Panthera leo")
	require.NoError(t, err)
	require.True(t, ok)

	assert.Equal(t, "Panthera leo", a.queries["taxa"].Get("q"))
	assert.Equal(t, "species", a.queries["taxa"].Get("rank"))
	assert.Equal(t, "41964", a.queries["observations"].Get("taxon_id"), "the taxon whose name is the species, not the first search hit")
	assert.Equal(t, "true", a.queries["observations"].Get("sounds"))
}

func TestClient_Sound_NeedsAName(t *testing.T) {
	t.Parallel()

	a := &api{}
	client := a.serve(t)

	_, ok, err := client.Sound(context.Background(), "  ")
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Empty(t, a.queries, "nothing to look up means nothing is asked")
}

func TestClient_Sound_GivesUpWhenTheContextDoes(t *testing.T) {
	t.Parallel()

	a := &api{taxa: fixture(t, "taxa_lion.json"), observations: fixture(t, "observations_lion_sounds.json")}
	client := a.serve(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, ok, err := client.Sound(ctx, "Panthera leo")
	require.Error(t, err)
	assert.False(t, ok)
}

// An anonymous client is the first thing a rate limiter penalises, so every
// call says whose it is.
func TestClient_Sound_SaysWhoIsAsking(t *testing.T) {
	t.Parallel()

	a := &api{taxa: fixture(t, "taxa_lion.json"), observations: fixture(t, "observations_lion_sounds.json")}
	client := a.serve(t)

	_, _, err := client.Sound(context.Background(), "Panthera leo")
	require.NoError(t, err)

	require.Len(t, a.agents, 2, "one call for the taxon, one for its sounds")
	for _, agent := range a.agents {
		assert.Equal(t, UserAgent, agent)
		assert.Contains(t, agent, "Sightings Safari")
		assert.NotContains(t, agent, "Go-http-client")
	}
}
