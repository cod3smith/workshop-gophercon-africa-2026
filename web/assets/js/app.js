// Small page manners: the bars on the final score, and the copy button
// under the shareable result. Both are decoration — the numbers they
// describe are already in the page as text.
(() => {
	"use strict";

	// The scores come from the server as a fraction, and are applied here so
	// the bars grow from nothing rather than appearing full. Without
	// JavaScript the bars stay empty and the numbers beside them still say
	// everything the bars would have.
	const fillBars = () => {
		document.querySelectorAll("[data-scale]").forEach((bar) => {
			bar.style.transform = `scaleX(${bar.dataset.scale})`;
		});
	};

	// On a task of its own rather than inside requestAnimationFrame, which
	// never fires while the tab is hidden: a page restored in a background
	// tab would otherwise sit there with every bar empty.
	setTimeout(fillBars, 0);

	document.querySelectorAll("[data-copy]").forEach((button) => {
		button.addEventListener("click", async () => {
			const source = document.getElementById(button.dataset.copy);
			if (!source || !navigator.clipboard) {
				return;
			}

			const label = button.textContent;
			try {
				await navigator.clipboard.writeText(source.textContent.trim());
				button.textContent = "Copied";
			} catch {
				button.textContent = "Select it and copy";
			}

			setTimeout(() => {
				button.textContent = label;
			}, 2000);
		});
	});
})();
