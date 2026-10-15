// Small page manners: the bars on the final score, and the copy button
// under the shareable result. Both are decoration — the numbers they
// describe are already in the page as text.
(() => {
	"use strict";

	// The widths come from the server as a percentage, and are applied here
	// so the bars animate in from zero rather than appearing full.
	requestAnimationFrame(() => {
		document.querySelectorAll("[data-width]").forEach((bar) => {
			bar.style.width = bar.dataset.width;
		});
	});

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
