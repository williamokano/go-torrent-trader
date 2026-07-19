import { afterEach, describe, expect, test } from "vitest";
import { getCaretCoordinates } from "./caretPosition";

function makeTextarea(value: string): HTMLTextAreaElement {
  const textarea = document.createElement("textarea");
  textarea.value = value;
  document.body.appendChild(textarea);
  return textarea;
}

afterEach(() => {
  document.body.innerHTML = "";
});

describe("getCaretCoordinates", () => {
  test("returns a top/left/height shape", () => {
    const textarea = makeTextarea("hello @al");
    const coords = getCaretCoordinates(textarea, 9);

    expect(coords).toEqual(
      expect.objectContaining({
        top: expect.any(Number),
        left: expect.any(Number),
        height: expect.any(Number),
      }),
    );
  });

  test("does not leave the mirror element attached to the DOM", () => {
    const textarea = makeTextarea("hello @al");
    getCaretCoordinates(textarea, 9);

    // Only the textarea itself should remain.
    expect(document.body.children).toHaveLength(1);
    expect(document.body.children[0]).toBe(textarea);
  });

  test("does not mutate the textarea's value or selection", () => {
    const textarea = makeTextarea("hello @al");
    textarea.setSelectionRange(3, 3);

    getCaretCoordinates(textarea, 9);

    expect(textarea.value).toBe("hello @al");
    expect(textarea.selectionStart).toBe(3);
  });

  test("handles a caret at the very end of the text", () => {
    const textarea = makeTextarea("hello");
    expect(() => getCaretCoordinates(textarea, 5)).not.toThrow();
  });

  test("handles an empty textarea", () => {
    const textarea = makeTextarea("");
    expect(() => getCaretCoordinates(textarea, 0)).not.toThrow();
  });
});
