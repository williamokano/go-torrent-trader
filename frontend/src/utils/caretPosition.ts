export interface CaretCoordinates {
  top: number;
  left: number;
  height: number;
}

// A plain <textarea> has no API for where a given character index renders,
// so there's no way to know where to anchor a popup like the @mention
// dropdown next to the caret. The standard workaround (used by e.g.
// component/textarea-caret-position) is a hidden clone of the textarea,
// styled identically so it wraps text exactly the same way, holding the
// value up to the caret plus a marker span — the span's offset in the clone
// is the caret's pixel position.
const MIRRORED_PROPERTIES = [
  "boxSizing",
  "width",
  "overflowX",
  "overflowY",
  "borderTopWidth",
  "borderRightWidth",
  "borderBottomWidth",
  "borderLeftWidth",
  "borderStyle",
  "paddingTop",
  "paddingRight",
  "paddingBottom",
  "paddingLeft",
  "fontStyle",
  "fontVariant",
  "fontWeight",
  "fontSize",
  "lineHeight",
  "fontFamily",
  "textAlign",
  "textTransform",
  "textIndent",
  "letterSpacing",
  "wordSpacing",
  "tabSize",
] as const satisfies readonly (keyof CSSStyleDeclaration)[];

/** The pixel position of `position` within `textarea`, relative to the textarea's own top-left corner. */
export function getCaretCoordinates(
  textarea: HTMLTextAreaElement,
  position: number,
): CaretCoordinates {
  const mirror = document.createElement("div");
  const computed = window.getComputedStyle(textarea);

  mirror.style.position = "absolute";
  mirror.style.visibility = "hidden";
  mirror.style.whiteSpace = "pre-wrap";
  mirror.style.wordWrap = "break-word";
  // Match the rendered content width (clientWidth excludes the scrollbar),
  // so long lines wrap at the same point the textarea itself wraps them.
  mirror.style.width = `${textarea.clientWidth}px`;

  for (const prop of MIRRORED_PROPERTIES) {
    mirror.style[prop] = computed[prop];
  }

  mirror.textContent = textarea.value.slice(0, position);
  const marker = document.createElement("span");
  // A trailing position needs *some* content to measure a caret-width box.
  marker.textContent = textarea.value.slice(position) || ".";
  mirror.appendChild(marker);

  document.body.appendChild(mirror);
  const coordinates: CaretCoordinates = {
    top: marker.offsetTop - textarea.scrollTop,
    left: marker.offsetLeft - textarea.scrollLeft,
    height: marker.offsetHeight,
  };
  document.body.removeChild(mirror);

  return coordinates;
}
