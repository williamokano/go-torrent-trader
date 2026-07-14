import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen } from "@testing-library/react";
import { MarkdownRenderer } from "./MarkdownRenderer";

describe("MarkdownRenderer", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("renders standard markdown", () => {
    const { container } = render(
      <MarkdownRenderer content={"# Title\n\n**bold** and *italic*"} />,
    );

    expect(screen.getByRole("heading", { name: "Title" })).toBeInTheDocument();
    expect(container.querySelector("strong")?.textContent).toBe("bold");
    expect(container.querySelector("em")?.textContent).toBe("italic");
  });

  it("renders GFM tables and strikethrough", () => {
    const { container } = render(
      <MarkdownRenderer
        content={"| a | b |\n| --- | --- |\n| 1 | 2 |\n\n~~gone~~"}
      />,
    );

    expect(container.querySelector("table")).toBeInTheDocument();
    expect(container.querySelector("del")?.textContent).toBe("gone");
  });

  describe("spoilers", () => {
    it("renders !!text!! as a click-to-reveal <details>", () => {
      const { container } = render(
        <MarkdownRenderer content="The butler !!did it!!" />,
      );

      const details = container.querySelector("details");
      expect(details).toBeInTheDocument();
      expect(details?.querySelector("summary")?.textContent).toBe("Spoiler");
      expect(details?.textContent).toContain("did it");
      // Closed by default — the content is only revealed on click.
      expect(details?.hasAttribute("open")).toBe(false);
    });

    it("keeps inline markdown inside the spoiler", () => {
      const { container } = render(
        <MarkdownRenderer content="!!**Ending**: he dies!!" />,
      );

      const details = container.querySelector("details");
      expect(details?.querySelector("strong")?.textContent).toBe("Ending");
      expect(details?.textContent).toContain("he dies");
    });

    it("renders several spoilers in one paragraph", () => {
      const { container } = render(
        <MarkdownRenderer content="!!one!! and !!two!!" />,
      );

      const details = container.querySelectorAll("details");
      expect(details).toHaveLength(2);
      expect(details[0].textContent).toContain("one");
      expect(details[1].textContent).toContain("two");
    });

    it("does not turn ordinary double exclamation marks into spoilers", () => {
      const { container } = render(
        <MarkdownRenderer content="Wow!! Amazing!!" />,
      );

      expect(container.querySelector("details")).not.toBeInTheDocument();
      expect(container.textContent).toContain("Wow!! Amazing!!");
    });

    it("leaves an unmatched delimiter as literal text", () => {
      const { container } = render(
        <MarkdownRenderer content="unbalanced !!spoiler" />,
      );

      expect(container.querySelector("details")).not.toBeInTheDocument();
      expect(container.textContent).toContain("unbalanced !!spoiler");
    });

    it("ignores delimiters inside code spans and code blocks", () => {
      const { container } = render(
        <MarkdownRenderer
          content={"`a !!b!! c`\n\n```\n!!not a spoiler!!\n```"}
        />,
      );

      expect(container.querySelector("details")).not.toBeInTheDocument();
      expect(container.textContent).toContain("!!b!!");
      expect(container.textContent).toContain("!!not a spoiler!!");
    });

    it("does not nest <details> inside a <p> (invalid DOM nesting)", () => {
      const errorSpy = vi.spyOn(console, "error").mockImplementation(() => {});

      const { container } = render(
        <MarkdownRenderer content="before !!secret!! after" />,
      );

      expect(container.querySelector("p details")).not.toBeInTheDocument();
      expect(container.querySelector("details")).toBeInTheDocument();
      // Surrounding text survives the paragraph -> div swap.
      expect(container.textContent).toContain("before");
      expect(container.textContent).toContain("after");
      expect(errorSpy).not.toHaveBeenCalled();
    });
  });

  describe("sanitization", () => {
    it("strips <script> tags from raw HTML", () => {
      const { container } = render(
        <MarkdownRenderer content={'Hi <script>alert("xss")</script> there'} />,
      );

      expect(container.querySelector("script")).not.toBeInTheDocument();
      expect(container.innerHTML).not.toContain("alert");
    });

    it("strips event handler attributes", () => {
      const { container } = render(
        <MarkdownRenderer content={'<img src="x" onerror="alert(1)" />'} />,
      );

      expect(
        container.querySelector("img")?.getAttribute("onerror"),
      ).toBeNull();
      expect(container.innerHTML).not.toContain("onerror");
    });

    it("strips javascript: URLs from links", () => {
      const { container } = render(
        <MarkdownRenderer content="[click](javascript:alert(1))" />,
      );

      const href = container.querySelector("a")?.getAttribute("href");
      expect(href ?? "").not.toContain("javascript:");
    });

    it("strips iframes and other unsafe tags", () => {
      const { container } = render(
        <MarkdownRenderer content='<iframe src="https://evil.example"></iframe>' />,
      );

      expect(container.querySelector("iframe")).not.toBeInTheDocument();
    });
  });

  describe("inline mode", () => {
    it("renders inline formatting inside a span", () => {
      const { container } = render(
        <MarkdownRenderer content="hello **world**" inline />,
      );

      expect(container.querySelector("span.markdown-body")).toBeInTheDocument();
      expect(container.querySelector("strong")?.textContent).toBe("world");
      expect(container.querySelector("p")).not.toBeInTheDocument();
    });

    it("drops block elements but keeps their text", () => {
      const { container } = render(
        <MarkdownRenderer
          content={"# Shouting\n\n![img](https://e.example/a.png)"}
          inline
        />,
      );

      expect(container.querySelector("h1")).not.toBeInTheDocument();
      expect(container.querySelector("img")).not.toBeInTheDocument();
      expect(container.textContent).toContain("Shouting");
    });

    it("still supports spoilers", () => {
      const { container } = render(
        <MarkdownRenderer content="!!hidden!!" inline />,
      );

      expect(container.querySelector("details")).toBeInTheDocument();
    });
  });
});
