package sightingsqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"uuid"
	"workshop/internal/domain/sighting"
	"workshop/internal/infra/db/sqlc"
	"workshop/internal/infra/db/sqlite"
)

// Store persists sightings using sqlc-generated SQLite queries.
type Store struct {
	queries db.Querier
}

var _ sighting.Store = (*Store)(nil)

// New constructs a sighting SQLite store.
func New(queries db.Querier) *Store {
	return &Store{queries: queries}
}

// Find returns the sighting for id, or sighting.ErrNotFound if it
// doesn't exist.
func (s Store) Find(ctx context.Context, id uuid.UUID) (sighting.Sighting, error) {
	row, err := s.queries.SightingFind(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return sighting.Sighting{}, sighting.ErrNotFound
	} else if err != nil {
		return sighting.Sighting{}, err
	}

	return transform(db.SightingListRow(row))
}

// FindByGbifKey returns the sighting for gbifKey, or
// sighting.ErrNotFound if it doesn't exist.
func (s Store) FindByGbifKey(ctx context.Context, gbifKey int) (sighting.Sighting, error) {
	row, err := s.queries.SightingFindByGbifKey(ctx, int64(gbifKey))
	if errors.Is(err, sql.ErrNoRows) {
		return sighting.Sighting{}, sighting.ErrNotFound
	} else if err != nil {
		return sighting.Sighting{}, err
	}

	return transform(db.SightingListRow(row))
}

// List returns the sightings matching filter, newest first. Sightings
// from the same moment are ordered by ID, so the order is total and the
// windows Limit and Offset cut from it never overlap.
func (s Store) List(ctx context.Context, filter sighting.ListFilter) ([]sighting.Sighting, error) {
	// The keys go in as one JSON array rather than through sqlc.slice, which
	// expands at runtime and throws SQLite's numbered placeholders out for
	// any argument after it, such as the limit.
	keys, err := json.Marshal(filter.TaxonKeys())
	if err != nil {
		return nil, err
	}

	// SQLite reads a negative limit as no limit, and will not take an
	// offset without one.
	limit := int64(filter.Limit)
	if limit <= 0 {
		limit = -1
	}

	rows, err := s.queries.SightingList(ctx, db.SightingListParams{
		FromDate:    filter.FromDate,
		ToDate:      filter.ToDate,
		CountryCode: filter.Country.String(),
		AnySpecies:  len(filter.Animals) == 0,
		SpeciesKeys: string(keys),
		Limit:       limit,
		Offset:      int64(max(filter.Offset, 0)),
	})
	if err != nil {
		return nil, err
	}

	// One line for the whole listing, with the count as an attribute, so
	// nothing is formatted unless debug logging is actually on. Formatting
	// a message per row — and the whole rows slice with it — made this
	// quadratic in the number of sightings.
	slog.DebugContext(ctx, "store: listing sightings", slog.Int("rows", len(rows)))

	result := make([]sighting.Sighting, len(rows))
	for i, row := range rows {
		result[i], err = transform(row)
		if err != nil {
			return nil, err
		}
	}

	return result, nil
}

// Count returns how many sightings match filter, whatever its Limit and
// Offset say.
func (s Store) Count(ctx context.Context, filter sighting.ListFilter) (int, error) {
	keys, err := json.Marshal(filter.TaxonKeys())
	if err != nil {
		return 0, err
	}

	n, err := s.queries.SightingCount(ctx, db.SightingCountParams{
		FromDate:    filter.FromDate,
		ToDate:      filter.ToDate,
		CountryCode: filter.Country.String(),
		AnySpecies:  len(filter.Animals) == 0,
		SpeciesKeys: string(keys),
	})
	if err != nil {
		return 0, err
	}

	return int(n), nil
}

// Totals returns the headline figures for the whole record.
//
// The first and last moments are read as typed columns rather than as the
// text MIN and MAX return: rows written by the seed and rows written since
// carry their time zone in different forms, and only the driver reads both.
func (s Store) Totals(ctx context.Context) (sighting.Totals, error) {
	row, err := s.queries.SightingTotals(ctx)
	if err != nil {
		return sighting.Totals{}, err
	}

	totals := sighting.Totals{
		Sightings: int(row.Sightings),
		Species:   int(row.Species),
		Countries: int(row.Countries),
		Media:     int(row.Media),
	}
	if totals.Sightings == 0 {
		return totals, nil
	}

	if totals.Earliest, err = s.queries.SightingEarliest(ctx); err != nil {
		return sighting.Totals{}, err
	}
	if totals.Latest, err = s.queries.SightingLatest(ctx); err != nil {
		return sighting.Totals{}, err
	}

	return totals, nil
}

// Create persists a new sighting under id, or returns
// sighting.ErrAlreadyExists if one is already there.
func (s Store) Create(ctx context.Context, id uuid.UUID, in sighting.CreateParams) (sighting.Sighting, error) {
	params, err := toSightingCreateParams(id, in)
	if err != nil {
		return sighting.Sighting{}, err
	}

	row, err := s.queries.SightingCreate(ctx, params)
	if sqlite.IsUniqueViolation(err) {
		return sighting.Sighting{}, sighting.ErrAlreadyExists
	} else if err != nil {
		return sighting.Sighting{}, err
	}

	// INSERT ... RETURNING cannot join, so the species and weather carry
	// only their IDs.
	return transform(db.SightingListRow{
		Sighting: row,
		Species:  db.Species{ID: row.SpeciesID},
		Weather:  db.Weather{ID: row.WeatherID},
	})
}
