import { useState } from "react";
import { cleanup, render, screen, fireEvent } from "@testing-library/react";
import { afterEach, beforeEach, describe, test, expect, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { setAccessToken } from "@/features/auth/token";
import { MarkdownEditor } from "./MarkdownEditor";

afterEach(cleanup);

/** Controlled harness — the editor pushes every change up to its parent. */
function Harness({ initial = "" }: { initial?: string }) {
  const [value, setValue] = useState(initial);
  return (
    <MemoryRouter>
      <MarkdownEditor label="Body" value={value} onChange={setValue} />
    </MemoryRouter>
  );
}

function setup(initial = "") {
  render(<Harness initial={initial} />);
  return screen.getByLabelText("Body") as HTMLTextAreaElement;
}

/** Places the caret / selection inside the textarea, like a real user would. */
function select(textarea: HTMLTextAreaElement, start: number, end = start) {
  textarea.focus();
  textarea.setSelectionRange(start, end);
}

describe("MarkdownEditor", () => {
  test("renders a plain textarea with the toolbar", () => {
    const textarea = setup("hello");

    expect(textarea.tagName).toBe("TEXTAREA");
    expect(textarea.value).toBe("hello");

    for (const name of [
      "Bold",
      "Italic",
      "Link",
      "Image",
      "Code",
      "Blockquote",
      "Spoiler (click to reveal)",
      "Bulleted list",
    ]) {
      expect(screen.getByRole("button", { name })).toBeInTheDocument();
    }
  });

  test("typing propagates to onChange", () => {
    const textarea = setup();
    fireEvent.change(textarea, { target: { value: "typed" } });
    expect(textarea.value).toBe("typed");
  });

  describe("toolbar insertion", () => {
    test("wraps the current selection", () => {
      const textarea = setup("hello world");
      select(textarea, 6, 11);

      fireEvent.click(screen.getByRole("button", { name: "Bold" }));

      expect(textarea.value).toBe("hello **world**");
    });

    test("inserts at the cursor when nothing is selected", () => {
      const textarea = setup("hello world");
      select(textarea, 5);

      fireEvent.click(screen.getByRole("button", { name: "Italic" }));

      expect(textarea.value).toBe("hello*italic text* world");
      // The placeholder is left selected so typing replaces it.
      expect(textarea.selectionStart).toBe(6);
      expect(textarea.selectionEnd).toBe(6 + "italic text".length);
    });

    test("inserts a spoiler", () => {
      const textarea = setup("the butler did it");
      select(textarea, 11, 17);

      fireEvent.click(
        screen.getByRole("button", { name: "Spoiler (click to reveal)" }),
      );

      expect(textarea.value).toBe("the butler !!did it!!");
    });

    test("inserts a link with a placeholder URL", () => {
      const textarea = setup("");
      select(textarea, 0);

      fireEvent.click(screen.getByRole("button", { name: "Link" }));

      expect(textarea.value).toBe("[link text](https://)");
    });

    test("inserts an image", () => {
      const textarea = setup("");
      select(textarea, 0);

      fireEvent.click(screen.getByRole("button", { name: "Image" }));

      expect(textarea.value).toBe("![alt text](https://)");
    });

    test("wraps a selection in inline code", () => {
      const textarea = setup("run npm ci now");
      select(textarea, 4, 10);

      fireEvent.click(screen.getByRole("button", { name: "Code" }));

      expect(textarea.value).toBe("run `npm ci` now");
    });

    test("prefixes every selected line for quotes", () => {
      const textarea = setup("first\nsecond\nthird");
      select(textarea, 0, 12);

      fireEvent.click(screen.getByRole("button", { name: "Blockquote" }));

      expect(textarea.value).toBe("> first\n> second\nthird");
    });

    test("prefixes the current line for lists", () => {
      const textarea = setup("one\ntwo");
      select(textarea, 5);

      fireEvent.click(screen.getByRole("button", { name: "Bulleted list" }));

      expect(textarea.value).toBe("one\n- two");
    });
  });

  describe("preview toggle", () => {
    test("renders the markdown and hides the textarea", () => {
      setup("**bold** and !!hidden!!");

      fireEvent.click(screen.getByRole("button", { name: "Preview" }));

      expect(screen.queryByLabelText("Body")).not.toBeInTheDocument();
      const preview = screen.getByTestId("markdown-preview");
      expect(preview.querySelector("strong")?.textContent).toBe("bold");
      expect(preview.querySelector("details")).toBeInTheDocument();
    });

    test("sanitizes XSS payloads in the preview", () => {
      setup('<script>alert("xss")</script><img src=x onerror="alert(1)">');

      fireEvent.click(screen.getByRole("button", { name: "Preview" }));

      const preview = screen.getByTestId("markdown-preview");
      expect(preview.querySelector("script")).not.toBeInTheDocument();
      expect(preview.innerHTML).not.toContain("onerror");
      expect(preview.innerHTML).not.toContain("alert");
    });

    test("shows a placeholder when there is nothing to preview", () => {
      setup("   ");

      fireEvent.click(screen.getByRole("button", { name: "Preview" }));

      expect(screen.getByText("Nothing to preview.")).toBeInTheDocument();
    });

    test("returns to the textarea with the content intact", () => {
      setup("draft text");

      fireEvent.click(screen.getByRole("button", { name: "Preview" }));
      fireEvent.click(screen.getByRole("button", { name: "Write" }));

      expect((screen.getByLabelText("Body") as HTMLTextAreaElement).value).toBe(
        "draft text",
      );
    });
  });

  describe("@mention autocomplete", () => {
    beforeEach(() => {
      vi.stubGlobal(
        "fetch",
        vi.fn(async () => ({
          ok: true,
          json: async () => ({
            users: [
              { id: 1, username: "alice" },
              { id: 2, username: "alan" },
            ],
          }),
        })),
      );
    });

    afterEach(() => {
      vi.unstubAllGlobals();
    });

    test("typing @ + a prefix searches and lists matching users", async () => {
      const textarea = setup();
      fireEvent.change(textarea, { target: { value: "@al" } });

      const options = await screen.findAllByRole("option");
      expect(options.map((o) => o.textContent)).toEqual(["@alice", "@alan"]);
      expect(fetch).toHaveBeenCalledWith(
        "http://localhost:8080/api/v1/users?search=al&per_page=8",
        expect.anything(),
      );
    });

    test("clicking a suggestion inserts @username at the caret", async () => {
      const textarea = setup();
      fireEvent.change(textarea, { target: { value: "@al" } });
      await screen.findAllByRole("option");

      fireEvent.mouseDown(screen.getByRole("option", { name: "@alice" }));

      expect(textarea.value).toBe("@alice ");
      expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
    });

    test("Enter selects the active suggestion", async () => {
      const textarea = setup();
      fireEvent.change(textarea, { target: { value: "@al" } });
      await screen.findAllByRole("option");

      fireEvent.keyDown(textarea, { key: "Enter" });

      expect(textarea.value).toBe("@alice ");
    });

    test("ArrowDown moves the active suggestion before selecting", async () => {
      const textarea = setup();
      fireEvent.change(textarea, { target: { value: "@al" } });
      await screen.findAllByRole("option");

      fireEvent.keyDown(textarea, { key: "ArrowDown" });
      fireEvent.keyDown(textarea, { key: "Enter" });

      expect(textarea.value).toBe("@alan ");
    });

    test("a mention after existing text keeps the preceding text", async () => {
      const textarea = setup();
      fireEvent.change(textarea, { target: { value: "hi @al" } });
      await screen.findAllByRole("option");

      fireEvent.keyDown(textarea, { key: "Enter" });

      expect(textarea.value).toBe("hi @alice ");
    });

    test("Escape closes the dropdown", async () => {
      const textarea = setup();
      fireEvent.change(textarea, { target: { value: "@al" } });
      await screen.findAllByRole("option");

      fireEvent.keyDown(textarea, { key: "Escape" });

      expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
    });

    test("a mid-word @ does not trigger the dropdown", async () => {
      const textarea = setup();
      fireEvent.change(textarea, { target: { value: "email@host" } });

      await new Promise((resolve) => setTimeout(resolve, 300));

      expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
      expect(fetch).not.toHaveBeenCalled();
    });

    test("no dropdown is shown in preview mode", async () => {
      const textarea = setup();
      fireEvent.change(textarea, { target: { value: "@al" } });
      await screen.findAllByRole("option");

      fireEvent.click(screen.getByRole("button", { name: "Preview" }));

      expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
    });

    test("Tab selects the active suggestion", async () => {
      const textarea = setup();
      fireEvent.change(textarea, { target: { value: "@al" } });
      await screen.findAllByRole("option");

      fireEvent.keyDown(textarea, { key: "Tab" });

      expect(textarea.value).toBe("@alice ");
    });

    test("blurring the textarea closes the dropdown", async () => {
      const textarea = setup();
      fireEvent.change(textarea, { target: { value: "@al" } });
      await screen.findAllByRole("option");

      fireEvent.blur(textarea);

      expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
    });

    test("completes the token at the caret without corrupting other text", async () => {
      const textarea = setup();
      // Two mention tokens; the dropdown opens for the second ("@al").
      fireEvent.change(textarea, { target: { value: "hey @bob and @al" } });
      await screen.findAllByRole("option");

      // Move the caret back onto the first token (@bob, caret at offset 8). The
      // dropdown stays open because the caret is still on a mention token.
      textarea.setSelectionRange(8, 8);
      fireEvent.select(textarea);

      fireEvent.keyDown(textarea, { key: "Enter" });

      // Completes @bob (the token the caret is actually in), leaves @al intact,
      // and inserts a single space — not the doubled space / duplicated tail a
      // stale start index would produce.
      expect(textarea.value).toBe("hey @alice and @al");
    });

    test("exposes combobox semantics tied to the active option", async () => {
      const textarea = setup();
      fireEvent.change(textarea, { target: { value: "@al" } });
      await screen.findAllByRole("option");

      expect(textarea).toHaveAttribute("role", "combobox");
      expect(textarea).toHaveAttribute("aria-expanded", "true");
      const active = screen.getByRole("option", { name: "@alice" });
      expect(textarea.getAttribute("aria-activedescendant")).toBe(active.id);
    });

    test("sends the bearer token when authenticated", async () => {
      setAccessToken("tok-123");
      try {
        const textarea = setup();
        fireEvent.change(textarea, { target: { value: "@al" } });
        await screen.findAllByRole("option");

        expect(fetch).toHaveBeenCalledWith(
          "http://localhost:8080/api/v1/users?search=al&per_page=8",
          { headers: { Authorization: "Bearer tok-123" } },
        );
      } finally {
        setAccessToken(null);
      }
    });

    test("drops a stale, out-of-order response", async () => {
      let resolveStale: () => void = () => {};
      const stale = new Promise((resolve) => {
        resolveStale = () =>
          resolve({
            ok: true,
            json: async () => ({ users: [{ id: 9, username: "stale" }] }),
          });
      });
      const fresh = Promise.resolve({
        ok: true,
        json: async () => ({ users: [{ id: 3, username: "alanis" }] }),
      });
      let call = 0;
      vi.stubGlobal(
        "fetch",
        vi.fn(() => (call++ === 0 ? stale : fresh)),
      );

      const textarea = setup();
      // First search fires (250ms debounce) and stays pending.
      fireEvent.change(textarea, { target: { value: "@al" } });
      await new Promise((r) => setTimeout(r, 300));
      // Second search fires and resolves, showing "alanis".
      fireEvent.change(textarea, { target: { value: "@ala" } });
      expect(
        await screen.findByRole("option", { name: "@alanis" }),
      ).toBeInTheDocument();

      // The first (now stale) response arrives last — the reqSeq guard drops it.
      resolveStale();
      await new Promise((r) => setTimeout(r, 50));

      expect(
        screen.queryByRole("option", { name: "@stale" }),
      ).not.toBeInTheDocument();
      expect(
        screen.getByRole("option", { name: "@alanis" }),
      ).toBeInTheDocument();
    });
  });
});
