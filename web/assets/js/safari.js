// The map a round is played on: an empty world to drop a pin on, or the
// guess and the answer side by side once the round has been scored.
//
// The map is an extra. Without it — or without Leaflet from the CDN — the
// page still says what it needs to, a guess can still be typed into the
// latitude and longitude fields, and every sighting is still readable on
// the record.

// Keeps the submit button honest about the two coordinate fields. It runs
// whether or not the map does, because typing is the way in that cannot
// depend on Leaflet loading.
//
// The button ships enabled so a page with no JavaScript can still submit;
// this is what turns it off while the fields can't be a guess. The browser
// does the judging (required, min, max and unparseable input all count as
// invalid), so there is no second set of rules here to drift from the markup.
(() => {
	"use strict";

	const lat = document.getElementById("lat");
	const lng = document.getElementById("lng");
	const submit = document.getElementById("guess-submit");
	if (!lat || !lng || !submit) {
		return;
	}

	const sync = () => {
		submit.disabled = !(lat.checkValidity() && lng.checkValidity());
	};

	lat.addEventListener("input", sync);
	lng.addEventListener("input", sync);
	sync();
})();

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

	// The fallback paragraph has done its job once Leaflet is here. A box
	// that ships hidden (the finished game's, whose list already says all
	// of this in words) is shown only now there is a map to put in it, and
	// before Leaflet measures it.
	box.textContent = "";
	box.hidden = false;

	const map = L.map(box, {
		minZoom: 1,
		worldCopyJump: true,
		zoomControl: data.mode !== "answer",
	});

	L.tileLayer("https://tile.openstreetmap.org/{z}/{x}/{y}.png", {
		maxZoom: 18,
		attribution: '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a>',
	}).addTo(map);

	// label wraps text for a tooltip. Leaflet assigns string content with
	// innerHTML, and these labels carry GBIF's contributor-authored place
	// names, so they go in as a text node that cannot execute.
	const label = (text) => {
		const span = document.createElement("span");
		span.textContent = text;
		return span;
	};

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
	} else if (data.mode === "final") {
		showFinal();
	} else {
		takeGuess();
	}

	// showAnswer draws the guess, the truth and the gap between them.
	function showAnswer() {
		const guess = L.latLng(data.guess.lat, data.guess.lng);
		const answer = L.latLng(data.answer.lat, data.answer.lng);

		pin(guess, gold).addTo(map).bindTooltip(label(data.guess.label));
		pin(answer, moss).addTo(map).bindTooltip(label(data.answer.label), { permanent: true, direction: "top" });

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

	// showFinal draws every round of a finished game: where each animal
	// really was, numbered as in the list beside it, and the pin the player
	// put against it. A line joins the two, so the five misses and the near
	// hits can be read at a glance. The map stays draggable, because five
	// places can be a continent apart.
	function showFinal() {
		const points = [];

		data.rounds.forEach((round) => {
			const answer = L.latLng(round.answer.lat, round.answer.lng);
			points.push(answer);

			// The guess is optional: a game begun before guesses were kept
			// has only its answers to show.
			if (round.guess) {
				const guess = L.latLng(round.guess.lat, round.guess.lng);
				points.push(guess);

				L.polyline([guess, answer], {
					color: gold,
					weight: 2,
					opacity: 0.6,
					dashArray: "5 7",
				}).addTo(map);
				pin(guess, gold).addTo(map).bindTooltip(label(round.guess.label));
			}

			// The number is permanent so the dot can be matched to its row;
			// the animal and place are a click away.
			pin(answer, moss)
				.addTo(map)
				.bindTooltip(label(String(round.n)), { permanent: true, direction: "top", offset: [0, -6] })
				.bindPopup(label(round.answer.label));
		});

		map.fitBounds(L.latLngBounds(points).pad(0.25));
	}

	// takeGuess lets the player drop a pin, and hands its position to the
	// form so the server scores a position it chose rather than a label.
	//
	// There are three ways in, because a map that only answers to a click
	// is a map only a mouse can play: click it, press Enter on it, or use
	// the button under it. The last two place the pin at the crosshair in
	// the middle, which the arrow keys move by panning the map. Typing the
	// coordinates works too, and moves the pin to match.
	function takeGuess() {
		const lat = document.getElementById("lat");
		const lng = document.getElementById("lng");
		const readout = document.getElementById("guess-readout");
		const crosshairReadout = document.getElementById("crosshair-readout");
		const frame = box.closest(".mapframe");
		const crosshair = frame && frame.querySelector("[data-crosshair]");
		const centre = document.querySelector("[data-drop-centre]");

		map.setView([12, 14], 2);

		// The typed route ships open so a page with no JavaScript, or no
		// Leaflet, has it. Only now that the map is built does it fold away.
		// Folded is not hidden: the summary stays focusable and the fields
		// stay in the accessibility tree, so it is still one keypress away.
		const typed = document.querySelector("details.coords");
		if (typed) {
			typed.open = false;
		}

		// The first-round greeting ships open. On a short phone, open it would
		// push the map under the pinned bar, so it folds to its summary strip,
		// still visible and one keypress from open. Only now that there is a
		// map to protect: with none, it stays open for the player who needs it.
		const welcome = document.querySelector("details.welcome");
		if (welcome && window.matchMedia("(max-width: 60rem) and (max-height: 52rem)").matches) {
			welcome.open = false;
		}

		// Both are useless without a live map, so the page ships them hidden
		// and they appear only once there is something to aim at.
		if (crosshair) {
			crosshair.hidden = false;
		}
		if (centre) {
			centre.hidden = false;
		}

		// The crosshair is only worth reading out once there is a map to aim
		// it with.
		const announceCrosshair = () => {
			if (!crosshairReadout) {
				return;
			}

			const point = map.getCenter().wrap();
			crosshairReadout.textContent = `Crosshair at ${point.lat.toFixed(3)}, ${point.lng.toFixed(3)}`;
		};

		if (crosshairReadout) {
			crosshairReadout.hidden = false;
			announceCrosshair();
		}

		// Panning fires moveend each time the map settles, and a screen
		// reader that is told about every one talks over itself. So the
		// readout is rewritten at most once every 400ms, and the update
		// always reads the centre as it is when it runs, which keeps the
		// last word accurate.
		const announceEvery = 400;
		let announceTimer = null;
		let announcedAt = 0;

		map.on("moveend", () => {
			if (announceTimer !== null) {
				return;
			}

			const wait = Math.max(0, announceEvery - (Date.now() - announcedAt));
			announceTimer = setTimeout(() => {
				announceTimer = null;
				announcedAt = Date.now();
				announceCrosshair();
			}, wait);
		});

		let marker = null;

		// show draws the pin and says where it is, without touching the
		// form: it is what both a click and a typed guess end in.
		const show = (point) => {
			if (marker === null) {
				marker = pin(point, gold).addTo(map);
			} else {
				marker.setLatLng(point);
			}

			// <output> is a live region, so this is also what a screen reader
			// hears the moment the pin lands.
			readout.textContent = `Pin at ${point.lat.toFixed(3)}, ${point.lng.toFixed(3)}`;
			readout.dataset.set = "true";
		};

		// place puts the pin down and tells the form where it went.
		const place = (latlng) => {
			// A click on a repeated copy of the world gives a longitude
			// outside ±180, which is nowhere on earth as far as the domain
			// is concerned, so it's wrapped before it goes anywhere.
			const point = latlng.wrap();

			lat.value = point.lat.toFixed(5);
			lng.value = point.lng.toFixed(5);
			// Setting a value from script fires nothing, and the submit
			// button listens for typing.
			lat.dispatchEvent(new Event("input"));
			lng.dispatchEvent(new Event("input"));

			show(point);
		};

		// A typed guess moves the pin, so the map and the fields never
		// disagree about where the guess is. "change" rather than "input":
		// a half-typed "-1." should not send the pin somewhere.
		const follow = () => {
			if (lat.checkValidity() && lng.checkValidity()) {
				show(L.latLng(Number(lat.value), Number(lng.value)));
			}
		};

		lat.addEventListener("change", follow);
		lng.addEventListener("change", follow);

		map.on("click", (event) => place(event.latlng));

		if (centre) {
			centre.addEventListener("click", () => place(map.getCenter()));
		}

		box.addEventListener("keydown", (event) => {
			if (event.key !== "Enter" && event.key !== " ") {
				return;
			}

			// Leaflet's zoom and attribution controls are links inside the
			// map; let their own keys through.
			if (event.target !== box) {
				return;
			}

			event.preventDefault();
			place(map.getCenter());
		});
	}
})();
