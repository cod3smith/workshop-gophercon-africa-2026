package pages

import (
	"fmt"
	"workshop/internal/domain/sighting"
)

// HomeProps is one page of the record, and what is needed to say where it
// sits in the whole.
type HomeProps struct {
	// Sightings is the page's tiles, newest first.
	Sightings []sighting.Sighting
	// Totals describes the whole record, whichever page this is.
	Totals sighting.Totals
	// Page is this page, counting from 1, and Pages how many there are.
	Page, Pages int
	// First is the place in the record of the page's first tile, counting
	// from 1.
	First int
}

// last is the place in the record of the page's last tile.
func (p HomeProps) last() int {
	return p.First + len(p.Sightings) - 1
}

// title names the page for the tab. Only the pages after the first are
// told apart, because the first is the front page.
func (p HomeProps) title() string {
	if p.Page <= 1 {
		return "The record"
	}
	return fmt.Sprintf("The record, page %d of %d", p.Page, p.Pages)
}

// pageHref is where page n of the record is. The first page is the front
// page itself, and every page lands on the record rather than the hero
// above it, so turning a page does not mean scrolling back down to it.
func pageHref(n int) string {
	if n <= 1 {
		return "/#record"
	}
	return fmt.Sprintf("/?page=%d#record", n)
}

// recordHref is where one sighting has a page of its own.
func recordHref(s sighting.Sighting) string {
	return "/record/" + s.ID.String()
}
