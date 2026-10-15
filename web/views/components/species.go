package components

import (
	"cmp"
	"slices"
	"strings"

	"workshop/internal/domain/sighting"
	"workshop/internal/domain/species"
)

// DisplayName prefers the name a reader would recognise, falling back
// through the taxonomy as GBIF records thin out.
func DisplayName(sp species.Species) string {
	return cmp.Or(sp.VernacularName, sp.CanonicalName, sp.ScientificName, "Unknown species")
}

// Place joins whichever parts of the location GBIF supplied.
func Place(l sighting.Location) string {
	parts := slices.DeleteFunc(
		[]string{l.Locality, l.StateProvince, l.Country},
		func(s string) bool { return s == "" },
	)
	return cmp.Or(strings.Join(parts, ", "), "Location unknown")
}
