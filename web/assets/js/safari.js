// The map a round is played on: an empty world to drop a pin on, or the
// guess and the answer side by side once the round has been scored.
//
// The map is an extra. Without it — or without Leaflet from the CDN — the
// page still says what it needs to, and every sighting is still readable
// on the record.
(() => {
	"use strict";

	const box = document.querySelector("[data-safari-map]");
	if (!box || typeof L === "undefined") {
		return;
	}

	const script = document.getElementById(box.dataset.safariMap);
	if (!script) {
		return;
	}

	const data = JSON.parse(script.textContent);
	const style = getComputedStyle(document.documentElement);
	const gold = style.getPropertyValue("--gold").trim() || "#d9a227";
	const moss = style.getPropertyValue("--moss").trim() || "#5fb37a";
	const ink = style.getPropertyValue("--ink").trim() || "#0b100e";

	// The fallback paragraph has done its job once Leaflet is here.
	box.textContent = "";

	const map = L.map(box, {
		minZoom: 1,
		worldCopyJump: true,
		zoomControl: data.mode === "guess",
	});

	L.tileLayer("https://tile.openstreetmap.org/{z}/{x}/{y}.png", {
		maxZoom: 18,
		attribution: '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a>',
	}).addTo(map);

	// pin returns a marker styled to the page's palette.
	const pin = (latlng, colour) =>
		L.circleMarker(latlng, {
			radius: 8,
			color: ink,
			weight: 2,
			fillColor: colour,
			fillOpacity: 1,
		});

	if (data.mode === "answer") {
		showAnswer();
	} else {
		takeGuess();
	}

	// showAnswer draws the guess, the truth and the gap between them.
	function showAnswer() {
		const guess = L.latLng(data.guess.lat, data.guess.lng);
		const answer = L.latLng(data.answer.lat, data.answer.lng);

		pin(guess, gold).addTo(map).bindTooltip(data.guess.label);
		pin(answer, moss).addTo(map).bindTooltip(data.answer.label, { permanent: true, direction: "top" });

		L.polyline([guess, answer], {
			color: gold,
			weight: 2,
			opacity: 0.8,
			dashArray: "5 7",
		}).addTo(map);

		map.fitBounds(L.latLngBounds([guess, answer]).pad(0.35));
		map.dragging.disable();
		map.scrollWheelZoom.disable();
		map.doubleClickZoom.disable();
	}

	// takeGuess lets the player drop a pin, and hands its position to the
	// form so the server scores a position it chose rather than a label.
	function takeGuess() {
		const lat = document.getElementById("lat");
		const lng = document.getElementById("lng");
		const submit = document.getElementById("guess-submit");
		const readout = document.getElementById("guess-readout");

		map.setView([12, 14], 2);

		let marker = null;

		map.on("click", (event) => {
			// Clicking a repeated copy of the world gives a longitude
			// outside ±180, which is nowhere on earth as far as the domain
			// is concerned, so it's wrapped before it goes anywhere.
			const point = event.latlng.wrap();

			if (marker === null) {
				marker = pin(point, gold).addTo(map);
			} else {
				marker.setLatLng(point);
			}

			lat.value = point.lat.toFixed(5);
			lng.value = point.lng.toFixed(5);

			submit.disabled = false;
			readout.textContent = `${point.lat.toFixed(3)}, ${point.lng.toFixed(3)}`;
			readout.dataset.set = "true";
		});
	}
})();
