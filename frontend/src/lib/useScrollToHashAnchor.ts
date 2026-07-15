import { useEffect, useRef } from "react";

/**
 * Scrolls the element whose id matches the current URL hash into view, once,
 * after the content that contains it has loaded. Deep-linkable paginated lists
 * (forum posts, torrent comments) load async, so the browser's native hash jump
 * fires before the anchor element exists — pass `ready` as a value that flips to
 * true once the list has rendered, and the scroll runs then.
 *
 * Best-effort: it only scrolls if the target is on the loaded page.
 */
export function useScrollToHashAnchor(ready: boolean): void {
  const scrolled = useRef(false);
  useEffect(() => {
    if (scrolled.current || !ready) return;
    const hash = window.location.hash;
    if (!hash) return;
    const el = document.getElementById(hash.slice(1));
    if (el) {
      scrolled.current = true;
      el.scrollIntoView({ behavior: "smooth", block: "start" });
    }
  }, [ready]);
}
