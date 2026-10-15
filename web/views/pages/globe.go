package pages

import (
	"cmp"
	"slices"
	"workshop/internal/domain/sighting"
)

type (
	// GlobeProps is the whole record, for the page that puts it on a sphere.
	// Everything the page shows is derived here, so the view does no
	// counting of its own.
	GlobeProps struct {
		Sightings []sighting.Sighting
	}
	// GlobeClass is one taxonomic class in the record: what to call it in
	// English, the Latin the colour is keyed by, and how many sightings
	// wear it. The legend is one row per class.
	GlobeClass struct {
		// Class is the Latin, which is what the stylesheet keys a colour by
		// and what the globe's points are coloured from.
		Class string
		Label string
		Count int
	}
	// GlobePoint is one sighting as the globe needs it: a position, the
	// names, the class its colour comes from, and the few words its tooltip
	// shows. It is deliberately not the whole Sighting: five hundred of
	// those is a payload nobody asked for.
	GlobePoint struct {
		ID             string  `json:"id"`
		Latitude       float64 `json:"latitude"`
		Longitude      float64 `json:"longitude"`
		Name           string  `json:"name"`
		ScientificName string  `json:"scientificName"`
		Class          string  `json:"class"`
		// Group is what the class is called in English, named here rather
		// than in the script, so the card on the globe calls a Crocodylia a
		// crocodile in the same words the list beside it does.
		Group   string `json:"group"`
		Country string `json:"country"`
		// Place is the whole place, as the list beside the globe prints it,
		// so a point and its row never disagree about where it was.
		Place string `json:"place"`
		// When is the date already written out in words rather than a
		// timestamp, because the only thing that reads it is a tooltip a
		// person looks at. Formatting it here and again in JavaScript is how
		// the two come to disagree.
		When      string `json:"when"`
		Thumbnail string `json:"thumbnail,omitzero"`
	}
)

// GlobePoints returns every sighting that has a position, in the order they
// were given, as the globe's feed.
//
// The result is never nil: the feed is an array, and a browser handed null
// fails at the first thing it tries to do with it.
func GlobePoints(sightings []sighting.Sighting) []GlobePoint {
	out := make([]GlobePoint, 0, len(sightings))

	for _, s := range sightings {
		if !hasPosition(s.Location) {
			continue
		}

		point := GlobePoint{
			ID:             s.ID.String(),
			Latitude:       s.Location.Latitude,
			Longitude:      s.Location.Longitude,
			Name:           speciesName(s),
			ScientificName: scientificLine(s),
			Class:          s.Species.Class,
			Group:          classLabel(s.Species.Class),
			Country:        cmp.Or(s.Location.Country, "Country unrecorded"),
			Place:          place(s.Location),
			When:           dateLabel(s),
		}

		// A tooltip is a hundred pixels wide, so it asks for the small copy
		// where the source keeps one.
		if image := photo(s); image.URL != "" {
			point.Thumbnail = PhotoURL(image, "small")
		}

		out = append(out, point)
	}

	return out
}

// classes are the taxonomic classes the record holds, commonest first and
// alphabetical where two of them tie, so the legend is the same list in the
// same order on every visit.
func (p GlobeProps) classes() []GlobeClass {
	counts := make(map[string]int, len(groups))
	for _, s := range p.Sightings {
		counts[s.Species.Class]++
	}

	out := make([]GlobeClass, 0, len(counts))
	for class, n := range counts {
		out = append(out, GlobeClass{Class: class, Label: classLabel(class), Count: n})
	}

	slices.SortFunc(out, func(a, b GlobeClass) int {
		return cmp.Or(cmp.Compare(b.Count, a.Count), cmp.Compare(a.Label, b.Label))
	})

	return out
}

// placed is how many sightings have a position, which is how many points
// the globe can actually hold. The rest are on the list and nowhere else,
// and the page says so rather than letting the two numbers disagree
// silently.
func (p GlobeProps) placed() int {
	n := 0
	for _, s := range p.Sightings {
		if hasPosition(s.Location) {
			n++
		}
	}
	return n
}

// unplaced is how many sightings nobody wrote a position for.
func (p GlobeProps) unplaced() int {
	return len(p.Sightings) - p.placed()
}

// classLabel names a taxonomic class the way the rest of the site does, and
// says plainly where the record never recorded one.
func classLabel(class string) string {
	if name, ok := groups[class]; ok {
		return name
	}
	if class != "" {
		return class
	}
	return "Unrecorded"
}

// globeDescription is the line a pasted link unfurls with.
func (p GlobeProps) globeDescription() string {
	return "Every sighting in the record on one globe: " +
		thousands(p.placed()) + " positions, coloured by what kind of animal was seen, each one a click from its page."
}
