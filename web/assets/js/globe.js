// The globe: every sighting in the record, on one sphere.
//
// The globe is an extra. The page it sits on lists every sighting as a link
// in the HTML the server sent, so the record is browsable with no WebGL, no
// three.js and no JavaScript at all. Nothing below writes a word of that
// list; it only lights up what is already there.
//
// Anything the mouse can do here, the keyboard can do through that list:
// every placed row carries a button that turns the globe to its point and
// opens the same card a click on the point opens, and the three buttons
// under the stage do the zooming. The canvas itself is a picture of the
// list, so it is aria-hidden and takes no focus.
//
// Everything is drawn with shaders of our own, so every colour below is
// sRGB exactly as the stylesheet wrote it: three.js only converts colour
// for its own materials, and THREE.Color would hand us the linear-space
// numbers that a shader writing straight to the framebuffer must not use.

(() => {
	"use strict";

	const stage = document.querySelector("[data-globe-stage]");
	if (!stage) {
		return;
	}

	const status = stage.querySelector("[data-globe-status]");
	const side = document.querySelector(".globe__side");
	const list = document.querySelector(".globe__list");
	const hint = document.querySelector("[data-globe-hint]");
	const controls = document.querySelector("[data-globe-controls]");
	const card = document.querySelector("[data-globe-card]");

	// say puts a sentence over the stage. It is the only thing that speaks
	// for the globe: before it is drawn, while it is drawing, and if it
	// cannot be. The element is removed outright once there is a globe.
	const say = (sentence) => {
		if (status && status.isConnected) {
			status.textContent = sentence;
		}
	};

	// A browser can have the script and still not get a context: an old
	// machine, a blocklist, a driver the browser has given up on. Asking is
	// the only way to know, and the context is handed straight back,
	// because a browser allows only so many and the one that matters is the
	// renderer's.
	const hasWebGL = () => {
		try {
			const probe = document.createElement("canvas");
			const gl = probe.getContext("webgl2") || probe.getContext("webgl");
			if (!gl) {
				return false;
			}

			const lose = gl.getExtension("WEBGL_lose_context");
			if (lose) {
				lose.loseContext();
			}
			return true;
		} catch {
			return false;
		}
	};

	if (typeof THREE === "undefined" || !hasWebGL()) {
		say(
			"The globe could not be drawn here: this browser has no WebGL, or the library " +
				"was blocked on its way in. Every sighting is listed beside it, and each one " +
				"links to its own page.",
		);
		return;
	}

	// Only now is this box going to hold a sphere, so only now does it take
	// a sphere's worth of the page. Where it never will, the sentence above
	// is all it is, and it is sized for a sentence.
	stage.classList.add("is-drawing");
	say("Drawing the globe…");

	/* --- Colours ---------------------------------------------------------
	   The stylesheet owns every colour on this page, including the six that
	   name the taxonomic classes. They are read from it rather than written
	   twice, so the point on the globe and the dot on its row in the list
	   cannot come to disagree. */

	const root = getComputedStyle(document.documentElement);
	const token = (name, fallback) => root.getPropertyValue(name).trim() || fallback;

	// rgb turns a CSS hex into the three numbers a shader wants.
	const rgb = (css) => {
		let hex = css.replace("#", "").trim();
		if (hex.length === 3) {
			hex = hex[0] + hex[0] + hex[1] + hex[1] + hex[2] + hex[2];
		}

		const n = Number.parseInt(hex, 16);
		if (hex.length !== 6 || Number.isNaN(n)) {
			return [0.6, 0.6, 0.6];
		}
		return [((n >> 16) & 255) / 255, ((n >> 8) & 255) / 255, (n & 255) / 255];
	};

	const ink = rgb(token("--ink", "#0d110f"));
	const ocean = rgb(token("--sunken", "#1b1f1d"));
	const coast = rgb(token("--edge", "#687971"));
	const grid = rgb(token("--line", "#3d4742"));
	const air = rgb(token("--moss", "#63bb7f"));
	const gold = rgb(token("--gold", "#d9a227"));
	const dim = token("--paper-dim", "#9bada2");

	// A class's colour is whatever the stylesheet says it is, read off the
	// first row in the list that wears it. One read per class, not one per
	// sighting, and no table of hexes in here to fall out of step.
	const classColours = new Map();
	const colourFor = (taxonomicClass) => {
		if (!classColours.has(taxonomicClass)) {
			const row = list && list.querySelector(`.globe__row[data-class="${CSS.escape(taxonomicClass)}"]`);
			const swatch = row ? getComputedStyle(row).getPropertyValue("--swatch").trim() : "";
			classColours.set(taxonomicClass, rgb(swatch || dim));
		}
		return classColours.get(taxonomicClass);
	};

	/* --- The scene ------------------------------------------------------- */

	const RADIUS = 1;
	const GRATICULE = RADIUS * 1.0025;
	const COAST = RADIUS * 1.004;
	const SURFACE = RADIUS * 1.016;
	const FIELD_OF_VIEW = 38;

	const scene = new THREE.Scene();
	// Near and far hug the scene: a unit sphere seen from between 1.45 and
	// 4.2 away. The coastline sits four thousandths of a radius above the
	// ocean, and a depth buffer stretched from 0.1 to 100 would not be able
	// to tell them apart.
	const camera = new THREE.PerspectiveCamera(FIELD_OF_VIEW, 1, 0.3, 12);
	camera.position.set(0, 0, 4);

	// preserveDrawingBuffer, because this globe is only drawn when something
	// moves. Without it the drawing buffer is the compositor's to throw away
	// after it has been shown, and a globe nobody is touching goes blank the
	// next time the page repaints.
	const renderer = new THREE.WebGLRenderer({ antialias: true, alpha: true, preserveDrawingBuffer: true });
	renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, 2));
	renderer.setClearAlpha(0);
	renderer.domElement.setAttribute("aria-hidden", "true");
	stage.appendChild(renderer.domElement);

	// The world turns; the light does not. Lighting in view space keeps the
	// sun over the viewer's left shoulder however far the globe is spun,
	// which is what makes it read as a sphere rather than a disc.
	const LIGHT = new THREE.Vector3(-0.55, 0.5, 0.67).normalize();

	const world = new THREE.Group();
	scene.add(world);

	const surfaceVertex = `
		varying vec3 vNormal;
		varying vec3 vEye;
		void main() {
			vec4 mv = modelViewMatrix * vec4(position, 1.0);
			vNormal = normalize(normalMatrix * normal);
			vEye = normalize(-mv.xyz);
			gl_Position = projectionMatrix * mv;
		}
	`;

	// The ocean: a terminator with a soft edge, and a rim where the sphere
	// turns away. The night side is lifted off black, because a dark ocean
	// on a near-black page is a hole rather than a planet.
	const ball = new THREE.Mesh(
		new THREE.SphereGeometry(RADIUS, 96, 64),
		new THREE.ShaderMaterial({
			uniforms: {
				uOcean: { value: new THREE.Vector3(...ocean) },
				uRim: { value: new THREE.Vector3(...air) },
				uLight: { value: LIGHT },
			},
			vertexShader: surfaceVertex,
			fragmentShader: `
				uniform vec3 uOcean;
				uniform vec3 uRim;
				uniform vec3 uLight;
				varying vec3 vNormal;
				varying vec3 vEye;
				void main() {
					float day = smoothstep(-0.15, 0.95, dot(vNormal, uLight));
					vec3 colour = uOcean * (0.35 + 2.05 * day);

					// The lit limb only. A rim all the way round reads as a ring
					// drawn on the page rather than as light falling on a ball.
					float rim = pow(1.0 - max(dot(vNormal, vEye), 0.0), 3.0);
					colour += uRim * rim * 0.38 * (0.12 + 0.88 * day);
					gl_FragColor = vec4(colour, 1.0);
				}
			`,
		}),
	);
	world.add(ball);

	// The air above it: the same sphere, a little larger, seen from the
	// inside and added to what is already there, so it only shows where the
	// globe's own silhouette stops. This is the halo.
	scene.add(
		new THREE.Mesh(
			new THREE.SphereGeometry(RADIUS * 1.14, 64, 48),
			new THREE.ShaderMaterial({
				uniforms: { uColour: { value: new THREE.Vector3(...air) }, uLight: { value: LIGHT } },
				vertexShader: surfaceVertex,
				fragmentShader: `
					uniform vec3 uColour;
					uniform vec3 uLight;
					varying vec3 vNormal;
					varying vec3 vEye;
					void main() {
						// Brightest where it meets the globe's own edge and gone
						// by the outside of this sphere, which is air thinning
						// out. The other way round is a ring drawn on the page.
						float edge = pow(abs(dot(vNormal, vEye)), 2.2);
						float day = smoothstep(-0.55, 0.85, dot(vNormal, uLight));
						gl_FragColor = vec4(uColour, edge * 1.5 * (0.22 + 0.78 * day));
					}
				`,
				transparent: true,
				depthWrite: false,
				side: THREE.BackSide,
				blending: THREE.AdditiveBlending,
			}),
		),
	);

	// flat is the material every line on the globe uses: one colour, no
	// lighting, and no depth of its own to fight the line beside it with.
	const flat = (colour, alpha) =>
		new THREE.ShaderMaterial({
			uniforms: { uColour: { value: new THREE.Vector3(...colour) }, uAlpha: { value: alpha } },
			vertexShader: `
				void main() {
					gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0);
				}
			`,
			fragmentShader: `
				uniform vec3 uColour;
				uniform float uAlpha;
				void main() {
					gl_FragColor = vec4(uColour, uAlpha);
				}
			`,
			transparent: true,
			depthWrite: false,
		});

	// onSphere is the whole of the map projection: a latitude and a
	// longitude become a point in the world.
	const onSphere = (latitude, longitude, radius) => {
		const phi = ((90 - latitude) * Math.PI) / 180;
		const theta = ((longitude + 180) * Math.PI) / 180;

		return [
			-radius * Math.sin(phi) * Math.cos(theta),
			radius * Math.cos(phi),
			radius * Math.sin(phi) * Math.sin(theta),
		];
	};

	// segments builds one geometry out of a list of polylines, so the whole
	// coastline of the world is a single thing to draw rather than three
	// hundred.
	const segments = (lines) => {
		let count = 0;
		for (const line of lines) {
			count += Math.max(line.length - 1, 0);
		}

		const positions = new Float32Array(count * 6);
		let at = 0;

		for (const line of lines) {
			for (let i = 1; i < line.length; i++) {
				positions.set(line[i - 1], at);
				positions.set(line[i], at + 3);
				at += 6;
			}
		}

		const geometry = new THREE.BufferGeometry();
		geometry.setAttribute("position", new THREE.BufferAttribute(positions, 3));
		return geometry;
	};

	// The graticule: a meridian every thirty degrees and a parallel every
	// thirty, which is enough to read the turn of the globe by and quiet
	// enough not to be read instead of the sightings.
	const graticule = () => {
		const lines = [];

		for (let longitude = -180; longitude < 180; longitude += 30) {
			const meridian = [];
			for (let latitude = -90; latitude <= 90; latitude += 3) {
				meridian.push(onSphere(latitude, longitude, GRATICULE));
			}
			lines.push(meridian);
		}

		for (let latitude = -60; latitude <= 60; latitude += 30) {
			const parallel = [];
			for (let longitude = -180; longitude <= 180; longitude += 3) {
				parallel.push(onSphere(latitude, longitude, GRATICULE));
			}
			lines.push(parallel);
		}

		return lines;
	};

	world.add(new THREE.LineSegments(segments(graticule()), flat(grid, 0.45)));

	/* --- The points ------------------------------------------------------
	   One geometry, one draw call, five hundred points. Each carries its own
	   colour and whether it is the one the card is open on; the shader does
	   the rest. */

	let points = [];
	let cloud = null;
	let lit = null;

	const pointMaterial = new THREE.ShaderMaterial({
		uniforms: {
			uSize: { value: 34 },
			uPixelRatio: { value: renderer.getPixelRatio() },
			uInk: { value: new THREE.Vector3(...ink) },
			uRing: { value: new THREE.Vector3(...gold) },
		},
		vertexShader: `
			attribute vec3 aColour;
			attribute float aLit;
			uniform float uSize;
			uniform float uPixelRatio;
			varying vec3 vColour;
			varying float vLit;
			varying float vFade;
			void main() {
				vec4 mv = modelViewMatrix * vec4(position, 1.0);

				// The globe is centred on the origin, so a point's own
				// position is also its normal. A point on the far side is
				// hidden by the depth test; one near the limb would pop in
				// and out as the globe turns, so it fades out instead.
				vec3 normal = normalize((modelViewMatrix * vec4(position, 0.0)).xyz);
				vFade = smoothstep(-0.03, 0.26, dot(normal, normalize(-mv.xyz)));

				vColour = aColour;
				vLit = aLit;
				gl_PointSize = uSize * uPixelRatio * (1.0 + aLit * 0.9) / max(-mv.z, 0.001);
				gl_Position = projectionMatrix * mv;
			}
		`,
		fragmentShader: `
			uniform vec3 uInk;
			uniform vec3 uRing;
			varying vec3 vColour;
			varying float vLit;
			varying float vFade;
			void main() {
				float r = length(gl_PointCoord - vec2(0.5)) * 2.0;
				if (r > 1.0 || vFade <= 0.0) {
					discard;
				}

				// A bright centre, a collar of the page's own near-black so
				// a pale dot still reads where it lands on a coastline, and
				// a glow that falls away to nothing at the edge.
				float core = 1.0 - smoothstep(0.30, 0.50, r);
				float collar = clamp((1.0 - smoothstep(0.52, 0.68, r)) - core, 0.0, 1.0);
				float glow = (1.0 - r) * 0.30 * (1.0 - core);
				float ring = vLit * (smoothstep(0.58, 0.68, r) - smoothstep(0.80, 0.90, r));

				vec3 colour = mix(vColour, uInk, collar);
				colour = mix(colour, uRing, clamp(ring, 0.0, 1.0));

				float alpha = core + glow + collar * 0.85 + clamp(ring, 0.0, 1.0);
				gl_FragColor = vec4(colour, clamp(alpha, 0.0, 1.0) * vFade);
			}
		`,
		transparent: true,
		depthWrite: false,
	});

	// plot turns the feed into the one geometry the globe draws.
	const plot = (sightings) => {
		const positions = new Float32Array(sightings.length * 3);
		const colours = new Float32Array(sightings.length * 3);
		lit = new Float32Array(sightings.length);

		sightings.forEach((sighting, i) => {
			positions.set(onSphere(sighting.latitude, sighting.longitude, SURFACE), i * 3);
			colours.set(colourFor(sighting.class || ""), i * 3);
		});

		const geometry = new THREE.BufferGeometry();
		geometry.setAttribute("position", new THREE.BufferAttribute(positions, 3));
		geometry.setAttribute("aColour", new THREE.BufferAttribute(colours, 3));
		geometry.setAttribute("aLit", new THREE.BufferAttribute(lit, 1));

		cloud = new THREE.Points(geometry, pointMaterial);
		world.add(cloud);
	};

	/* --- Turning it ------------------------------------------------------ */

	// The globe is turned by rotating the world, not by flying the camera
	// round it: two angles and a distance are the whole of the view, which
	// makes turning to a given sighting a sum rather than a search.
	const TILT_LIMIT = 1.45;
	const NEAREST = 1.7;
	const FURTHEST = 6.5;

	// Africa, a little below the equator: where most of this record was
	// recorded, and so where it opens, and where "reset the view" returns to.
	const OPEN_TILT = -0.09;
	const OPEN_TURN = -2.0;
	const OPEN_DISTANCE = 4.0;

	let tilt = OPEN_TILT;
	let turn = OPEN_TURN;
	let distance = OPEN_DISTANCE;

	let spin = false;
	const SPIN = 0.045; // radians a second: a turn of the earth in rather over two minutes

	const still = window.matchMedia("(prefers-reduced-motion: reduce)");

	let glide = null;
	let frame = null;
	let drawnAt = 0;

	// Set if the context is ever taken away: there is no globe to draw or
	// measure after that, only the list.
	let dead = false;

	const invalidate = () => {
		if (frame === null && !dead && !document.hidden) {
			frame = window.requestAnimationFrame(draw);
		}
	};

	// The loop runs only while something is moving. Nothing moving means no
	// frame is asked for at all, so a globe sitting still, or a tab in the
	// background, costs nothing.
	const draw = (now) => {
		frame = null;

		const elapsed = drawnAt === 0 ? 0 : Math.min((now - drawnAt) / 1000, 0.1);
		drawnAt = now;

		let moving = false;

		if (glide !== null) {
			const through = Math.min((now - glide.from) / glide.over, 1);
			const eased = 1 - (1 - through) ** 3;

			tilt = glide.tilt[0] + (glide.tilt[1] - glide.tilt[0]) * eased;
			turn = glide.turn[0] + (glide.turn[1] - glide.turn[0]) * eased;
			distance = glide.distance[0] + (glide.distance[1] - glide.distance[0]) * eased;

			if (through >= 1) {
				glide = null;
			} else {
				moving = true;
			}
		} else if (spin && !still.matches) {
			turn += SPIN * elapsed;
			moving = true;
		}

		world.rotation.set(tilt, turn, 0);
		camera.position.z = distance;
		renderer.render(scene, camera);

		if (moving) {
			invalidate();
		} else {
			drawnAt = 0;
		}
	};

	// turnTo brings a sighting to the front of the globe. The maths is the
	// projection run backwards: rotate about y until the point is on the
	// plane the camera looks down, then about x until it is on the axis.
	const turnTo = (sighting) => {
		const [x, y, z] = onSphere(sighting.latitude, sighting.longitude, RADIUS);
		const horizon = Math.sqrt(x * x + z * z);

		const wantTurn = Math.atan2(-x, z);
		const wantTilt = Math.max(Math.min(Math.atan2(y, horizon), TILT_LIMIT), -TILT_LIMIT);

		// The short way round: a sighting two degrees west is two degrees
		// west, not three hundred and fifty-eight degrees east.
		const shortest = turn + ((((wantTurn - turn) % (Math.PI * 2)) + Math.PI * 3) % (Math.PI * 2)) - Math.PI;
		const wantDistance = Math.min(distance, 2.4);

		spin = false;

		if (still.matches) {
			tilt = wantTilt;
			turn = shortest;
			distance = wantDistance;
			glide = null;
			invalidate();
			return;
		}

		glide = {
			from: performance.now(),
			over: 750,
			tilt: [tilt, wantTilt],
			turn: [turn, shortest],
			distance: [distance, wantDistance],
		};
		invalidate();
	};

	const zoom = (by) => {
		spin = false;
		glide = null;
		distance = Math.max(NEAREST, Math.min(FURTHEST, distance + by));
		invalidate();
	};

	/* --- The card -------------------------------------------------------- */

	const slots = {
		photo: card && card.querySelector("[data-globe-card-photo]"),
		name: card && card.querySelector("[data-globe-card-name]"),
		scientific: card && card.querySelector("[data-globe-card-scientific]"),
		meta: card && card.querySelector("[data-globe-card-meta]"),
		link: card && card.querySelector("[data-globe-card-link]"),
		close: card && card.querySelector("[data-globe-card-close]"),
	};

	// Where each sighting is in the feed, and which row in the list is its.
	const feedIndex = new Map();
	const rows = new Map();

	let chosen = -1;
	let openedBy = null;

	// highlight is the ring on the globe, and nothing else: the row the
	// pointer is over borrows it, and gives it back on the way out.
	const highlight = (index) => {
		if (lit === null || cloud === null) {
			return;
		}

		lit.fill(0);
		if (index >= 0 && index < lit.length) {
			lit[index] = 1;
		}

		cloud.geometry.getAttribute("aLit").needsUpdate = true;
		invalidate();
	};

	const closeCard = ({ restoreFocus = false } = {}) => {
		if (!card || card.hidden) {
			return;
		}

		card.hidden = true;
		if (chosen >= 0) {
			const row = rows.get(points[chosen].id);
			if (row) {
				row.element.classList.remove("is-selected");
				row.button.setAttribute("aria-expanded", "false");
			}
		}

		chosen = -1;
		highlight(-1);

		if (restoreFocus && openedBy && openedBy.isConnected) {
			openedBy.focus();
		}
		openedBy = null;
	};

	// open names one sighting: the card beside the globe, the ring on its
	// point, and the row in the list marked so the two read as one thing.
	// The keyboard asked for it through the row, so the card takes focus
	// when it did, and nothing moves when the mouse did.
	const open = (index, { byKeyboard = false } = {}) => {
		if (!card || index < 0 || index >= points.length) {
			return;
		}

		closeCard();

		const sighting = points[index];
		chosen = index;

		slots.name.textContent = sighting.name;
		slots.scientific.textContent = sighting.scientificName;
		slots.meta.textContent = [sighting.place, sighting.when, sighting.group].filter(Boolean).join(" · ");
		slots.link.setAttribute("href", "/record/" + encodeURIComponent(sighting.id));

		if (sighting.thumbnail) {
			slots.photo.src = sighting.thumbnail;
			slots.photo.alt = sighting.name;
			slots.photo.hidden = false;
		} else {
			slots.photo.removeAttribute("src");
			slots.photo.alt = "";
			slots.photo.hidden = true;
		}

		card.hidden = false;
		highlight(index);

		const row = rows.get(sighting.id);
		if (row) {
			row.element.classList.add("is-selected");
			row.button.setAttribute("aria-expanded", "true");
			row.element.scrollIntoView({ block: "nearest", behavior: still.matches ? "auto" : "smooth" });
		}

		if (byKeyboard) {
			openedBy = row ? row.button : null;
			card.focus();
		}
	};

	/* --- Picking --------------------------------------------------------- */

	const ray = new THREE.Raycaster();
	const pointer = new THREE.Vector2();
	const spot = new THREE.Vector3();
	const toEye = new THREE.Vector3();

	// A point is drawn uSize CSS pixels across at one unit of depth, and
	// shrinks with distance exactly as the world does, so the width it
	// covers on the globe is the same wherever the camera is. Hence a
	// threshold that depends on the size of the viewport and not on the
	// zoom, with a little room over for a finger.
	const threshold = () => {
		const height = renderer.domElement.clientHeight || 1;
		return (1.6 * pointMaterial.uniforms.uSize.value * Math.tan((FIELD_OF_VIEW * Math.PI) / 360)) / height;
	};

	// facing reports whether a point is on the half of the globe turned
	// towards us. The raycaster will happily find one on the far side,
	// which is behind an opaque sphere and so not a thing anybody clicked.
	const facing = (index) => {
		spot.fromBufferAttribute(cloud.geometry.getAttribute("position"), index).applyMatrix4(world.matrixWorld);
		toEye.copy(camera.position).sub(spot).normalize();
		return spot.normalize().dot(toEye);
	};

	const pick = (clientX, clientY) => {
		if (cloud === null) {
			return -1;
		}

		const box = renderer.domElement.getBoundingClientRect();
		pointer.set(((clientX - box.left) / box.width) * 2 - 1, -((clientY - box.top) / box.height) * 2 + 1);

		ray.params.Points.threshold = threshold();
		ray.setFromCamera(pointer, camera);

		// Nearest first, so the first one in front is the one that was hit.
		for (const hit of ray.intersectObject(cloud, false)) {
			if (facing(hit.index) > 0.03) {
				return hit.index;
			}
		}

		return -1;
	};

	/* --- Hands on it ----------------------------------------------------- */

	const canvas = renderer.domElement;
	const touches = new Map();
	let dragging = false;
	let travelled = 0;
	let pinch = 0;

	canvas.addEventListener("pointerdown", (event) => {
		touches.set(event.pointerId, { x: event.clientX, y: event.clientY });
		canvas.setPointerCapture(event.pointerId);

		// Any hand on it stops the globe turning by itself. It never starts
		// again: a thing that resumes drifting under the cursor is a thing
		// you cannot aim at.
		spin = false;
		glide = null;

		if (touches.size === 1) {
			dragging = true;
			travelled = 0;
			stage.classList.add("is-turning");
		} else {
			dragging = false;
			pinch = 0;
		}
	});

	canvas.addEventListener("pointermove", (event) => {
		const was = touches.get(event.pointerId);
		if (!was) {
			return;
		}

		touches.set(event.pointerId, { x: event.clientX, y: event.clientY });

		// Two fingers is a pinch, which zooms, and never a turn.
		if (touches.size >= 2) {
			const [a, b] = [...touches.values()];
			const apart = Math.hypot(a.x - b.x, a.y - b.y);

			if (pinch > 0) {
				distance = Math.max(NEAREST, Math.min(FURTHEST, distance * (pinch / Math.max(apart, 1))));
				invalidate();
			}
			pinch = apart;
			return;
		}

		if (!dragging) {
			return;
		}

		const byX = event.clientX - was.x;
		const byY = event.clientY - was.y;
		travelled += Math.abs(byX) + Math.abs(byY);

		// Scaled by the viewport, so a drag across the globe turns it by as
		// much on a phone as on a desk, and by the distance, so a close-up
		// is not flung about.
		const across = (distance / 4.0) * (2.4 / Math.max(canvas.clientWidth, 1));
		turn += byX * across * Math.PI;
		tilt = Math.max(-TILT_LIMIT, Math.min(TILT_LIMIT, tilt + byY * across * Math.PI));
		invalidate();
	});

	const letGo = (event) => {
		const was = touches.get(event.pointerId);
		touches.delete(event.pointerId);

		if (touches.size < 2) {
			pinch = 0;
		}
		if (touches.size === 0) {
			stage.classList.remove("is-turning");
		}

		if (!dragging || !was) {
			return;
		}
		dragging = false;

		// A drag is not a click. Six pixels is the slip in a hand that
		// meant to tap.
		if (travelled > 6) {
			return;
		}

		const index = pick(event.clientX, event.clientY);
		if (index < 0) {
			closeCard();
			return;
		}
		open(index);
	};

	canvas.addEventListener("pointerup", letGo);
	canvas.addEventListener("pointercancel", (event) => {
		touches.delete(event.pointerId);
		dragging = false;
		pinch = 0;
		stage.classList.remove("is-turning");
	});

	canvas.addEventListener(
		"wheel",
		(event) => {
			event.preventDefault();
			zoom(event.deltaY * 0.0013 * Math.max(distance, 1));
		},
		{ passive: false },
	);

	if (controls) {
		controls.addEventListener("click", (event) => {
			const button = event.target.closest("[data-globe-zoom], [data-globe-reset]");
			if (!button) {
				return;
			}

			if (button.hasAttribute("data-globe-reset")) {
				closeCard();
				tilt = OPEN_TILT;
				turn = OPEN_TURN;
				distance = OPEN_DISTANCE;
				spin = false;
				glide = null;
				invalidate();
				return;
			}

			zoom(button.dataset.globeZoom === "in" ? -0.45 : 0.45);
		});
	}

	if (slots.close) {
		slots.close.addEventListener("click", () => closeCard({ restoreFocus: true }));
	}

	if (card) {
		card.addEventListener("keydown", (event) => {
			if (event.key === "Escape") {
				event.stopPropagation();
				closeCard({ restoreFocus: true });
			}
		});
	}

	// The list is the keyboard's way onto the globe. One listener, not five
	// hundred.
	if (list) {
		list.addEventListener("click", (event) => {
			const button = event.target.closest("[data-globe-locate]");
			if (!button) {
				return;
			}

			const index = feedIndex.get(button.dataset.globeLocate);
			if (index === undefined) {
				return;
			}

			if (chosen === index && card && !card.hidden) {
				closeCard({ restoreFocus: true });
				return;
			}

			turnTo(points[index]);
			open(index, { byKeyboard: true });
		});

		// Running the eye down the list lights the point it is on, which is
		// how you find out where a row is without leaving the list. It
		// borrows the ring and gives it back.
		list.addEventListener("pointerover", (event) => {
			const row = event.target.closest(".globe__row");
			const id = row && row.querySelector("[data-globe-locate]");
			if (!id || chosen >= 0) {
				return;
			}

			const index = feedIndex.get(id.dataset.globeLocate);
			if (index !== undefined) {
				highlight(index);
			}
		});

		list.addEventListener("pointerleave", () => {
			if (chosen < 0) {
				highlight(-1);
			}
		});
	}

	window.addEventListener("keydown", (event) => {
		if (event.key === "Escape") {
			closeCard({ restoreFocus: true });
		}
	});

	// A hidden tab gets no frames at all, and the clock is forgotten so the
	// globe does not leap forward on the way back.
	document.addEventListener("visibilitychange", () => {
		if (document.hidden) {
			if (frame !== null) {
				window.cancelAnimationFrame(frame);
				frame = null;
			}
			drawnAt = 0;
			return;
		}
		invalidate();
	});

	still.addEventListener("change", () => {
		if (still.matches) {
			spin = false;
			glide = null;
		}
		invalidate();
	});

	// A context can be taken away: a laptop lid, a driver reset, too many
	// canvases. The list is still the record, so the page says so.
	canvas.addEventListener("webglcontextlost", (event) => {
		event.preventDefault();
		dead = true;
		spin = false;
		glide = null;
		closeCard();
		canvas.remove();
		stage.classList.remove("is-live", "is-turning");
		stage.insertAdjacentHTML(
			"beforeend",
			'<p class="globe__fallback">The globe stopped being drawable in this browser. Every sighting is still listed beside it.</p>',
		);
	});

	const fit = () => {
		const width = stage.clientWidth;
		const height = stage.clientHeight;
		if (dead || width <= 0 || height <= 0) {
			return;
		}

		renderer.setSize(width, height);
		pointMaterial.uniforms.uPixelRatio.value = renderer.getPixelRatio();
		camera.aspect = width / height;
		camera.updateProjectionMatrix();
		invalidate();
	};

	new ResizeObserver(fit).observe(stage);
	fit();

	/* --- What goes on it -------------------------------------------------- */

	// world-atlas keeps the land as TopoJSON: a transform, and arcs of
	// delta-encoded whole numbers. Every arc in land-110m is a stretch of
	// coast, so the coastline is the arcs themselves — there is no need to
	// stitch them back into polygons, and so no need for a second library
	// to do it. Pinned to one version, which cannot change under us; it is
	// read as numbers and never as code, and a globe that never receives it
	// keeps its graticule, its sightings and its list.
	//
	// Served from this site rather than a CDN. It is 55KB, and this is shown
	// on conference wifi: a third-party fetch that times out would leave the
	// globe without a coastline, which is most of what makes it readable.
	// The file is world-atlas 2.0.2's land-110m.json, built from Natural
	// Earth, which is public domain.
	const ATLAS = "/assets/data/land-110m.json";

	const coastline = (topo) => {
		const { scale, translate } = topo.transform;

		return topo.arcs.map((arc) => {
			let x = 0;
			let y = 0;

			return arc.map(([dx, dy]) => {
				x += dx;
				y += dy;
				return onSphere(y * scale[1] + translate[1], x * scale[0] + translate[0], COAST);
			});
		});
	};

	fetch(ATLAS)
		.then((response) => (response.ok ? response.json() : Promise.reject(new Error(String(response.status)))))
		.then((topo) => {
			if (!topo || !topo.transform || !Array.isArray(topo.arcs)) {
				return;
			}

			world.add(new THREE.LineSegments(segments(coastline(topo)), flat(coast, 0.85)));
			invalidate();
		})
		.catch(() => {
			// A globe with no coastline is still a globe with a graticule and
			// five hundred sightings on it, which is what the page is for.
		});

	fetch(stage.dataset.globeFeed, { headers: { Accept: "application/json" } })
		.then((response) => (response.ok ? response.json() : Promise.reject(new Error(String(response.status)))))
		.then((feed) => {
			if (!Array.isArray(feed)) {
				throw new TypeError("the feed is not a list of sightings");
			}

			points = feed.filter((s) => Number.isFinite(s.latitude) && Number.isFinite(s.longitude));
			if (points.length === 0) {
				say("There is nothing on the record to put on a globe yet.");
				return;
			}

			points.forEach((sighting, i) => feedIndex.set(sighting.id, i));
			plot(points);

			// Everything that only makes sense with a globe behind it is
			// shown now, and not a moment before.
			if (list) {
				for (const button of list.querySelectorAll("[data-globe-locate]")) {
					const element = button.closest(".globe__row");
					if (feedIndex.has(button.dataset.globeLocate) && element) {
						button.hidden = false;
						rows.set(button.dataset.globeLocate, { element, button });
					}
				}
			}

			if (side) {
				side.classList.add("is-live");
			}
			if (hint) {
				hint.hidden = false;
			}
			if (controls) {
				controls.hidden = false;
			}

			stage.classList.add("is-live");
			if (status) {
				status.remove();
			}

			spin = !still.matches;
			invalidate();
		})
		.catch(() => {
			say(
				"The sightings could not be loaded, so there is nothing on the globe. Every one " +
					"of them is listed beside it, and each one links to its own page.",
			);
		});
})();
