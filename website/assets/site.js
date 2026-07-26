// Fills the piece grid the way a swarm actually fills one: out of order, in bursts,
// with a few pieces marked freeleech. Decorative, so it degrades to a static grid
// when JS is off or motion is reduced.
(function () {
  "use strict";

  var reduced =
    window.matchMedia &&
    window.matchMedia("(prefers-reduced-motion: reduce)").matches;

  document.querySelectorAll("[data-pieces]").forEach(function (list) {
    var total = parseInt(list.dataset.pieces, 10) || 96;
    var freeEvery = parseInt(list.dataset.free, 10) || 0;

    var order = [];
    for (var i = 0; i < total; i++) {
      var li = document.createElement("li");
      list.appendChild(li);
      order.push(i);
    }
    var items = list.children;

    // Fisher-Yates, so arrival order is genuinely scattered rather than a sweep.
    for (var j = order.length - 1; j > 0; j--) {
      var k = Math.floor(Math.random() * (j + 1));
      var t = order[j];
      order[j] = order[k];
      order[k] = t;
    }

    var have = Math.floor(total * 0.72);

    if (reduced) {
      for (var r = 0; r < have; r++) {
        items[order[r]].className =
          freeEvery && r % freeEvery === 0 ? "free" : "have";
      }
      return;
    }

    var n = 0;
    (function tick() {
      if (n >= have) return;
      var burst = 1 + Math.floor(Math.random() * 3);
      for (var b = 0; b < burst && n < have; b++, n++) {
        items[order[n]].className =
          freeEvery && n % freeEvery === 0 ? "free" : "have";
      }
      setTimeout(tick, 40 + Math.random() * 90);
    })();
  });

  // Divider strips: cheap, static, no timers.
  document.querySelectorAll("[data-strip]").forEach(function (strip) {
    var n = parseInt(strip.dataset.strip, 10) || 40;
    for (var i = 0; i < n; i++) strip.appendChild(document.createElement("span"));
  });
})();
