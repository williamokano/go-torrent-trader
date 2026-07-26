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

    // A release named `Show !!secret ending!! 01` used to rewrite the link label,
    // producing <a>Show </a><details>…</details><a> 01</a> — a block-level widget
    // inside what the shoutbox renders as a single row, with the link torn into
    // three pieces. Escaping cannot fix it from the caller's side: the plugin runs
    // on the mdast after escapes have resolved.
    it("leaves !! alone inside a link label", () => {
      const { container } = render(
        <MarkdownRenderer content="[Show !!secret ending!! 01](https://example.com/t/1)" />,
      );

      const links = container.querySelectorAll("a");
      expect(links).toHaveLength(1);
      expect(links[0].textContent).toBe("Show !!secret ending!! 01");
      expect(container.querySelector("details")).toBeNull();
    });

    it("still renders a spoiler alongside a link in the same paragraph", () => {
      const { container } = render(
        <MarkdownRenderer content="see [the page](https://example.com) and !!the twist!!" />,
      );

      expect(container.querySelectorAll("a")).toHaveLength(1);
      expect(container.querySelector("details")?.textContent).toContain(
        "the twist",
      );
    });

    it("still allows a link inside a spoiler", () => {
      const { container } = render(
        <MarkdownRenderer content="!!it is [here](https://example.com)!!" />,
      );

      const details = container.querySelector("details");
      expect(details).toBeInTheDocument();
      expect(details?.querySelector("a")?.textContent).toBe("here");
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

  describe("mentions", () => {
    it("links a resolved mention to the user's profile", () => {
      const { container } = render(
        <MarkdownRenderer
          content="great work @alice"
          mentionedUsernames={["alice"]}
        />,
      );

      const link = container.querySelector("a.mention");
      expect(link).toBeInTheDocument();
      expect(link?.getAttribute("href")).toBe("/user/alice");
      expect(link?.textContent).toBe("@alice");
    });

    it("leaves an unresolved token as plain text", () => {
      const { container } = render(
        <MarkdownRenderer content="hey @typo" mentionedUsernames={["alice"]} />,
      );

      expect(container.querySelector("a.mention")).not.toBeInTheDocument();
      expect(container.textContent).toContain("@typo");
    });

    it("renders plain text when no mentions were resolved", () => {
      const { container } = render(
        <MarkdownRenderer content="hey @alice" mentionedUsernames={[]} />,
      );

      expect(container.querySelector("a.mention")).not.toBeInTheDocument();
      expect(container.textContent).toContain("@alice");
    });

    it("does not linkify when mentionedUsernames is omitted", () => {
      const { container } = render(<MarkdownRenderer content="hey @alice" />);

      expect(container.querySelector("a.mention")).not.toBeInTheDocument();
      expect(container.textContent).toContain("@alice");
    });

    it("never linkifies an email-shaped token, even if the tail matches a valid mention", () => {
      // "alice" is a real, resolvable username, but "x@alice" is not a
      // mention — same left-boundary rule the backend enforces.
      const { container } = render(
        <MarkdownRenderer
          content="contact me at x@alice for details"
          mentionedUsernames={["alice"]}
        />,
      );

      expect(container.querySelector("a.mention")).not.toBeInTheDocument();
      expect(container.textContent).toContain("x@alice");
    });

    it("links multiple distinct mentions in the same content", () => {
      const { container } = render(
        <MarkdownRenderer
          content="@alice and @bob, take a look"
          mentionedUsernames={["alice", "bob"]}
        />,
      );

      const links = container.querySelectorAll("a.mention");
      expect(links).toHaveLength(2);
      expect(links[0].textContent).toBe("@alice");
      expect(links[0].getAttribute("href")).toBe("/user/alice");
      expect(links[1].textContent).toBe("@bob");
      expect(links[1].getAttribute("href")).toBe("/user/bob");
    });

    it("links a mention at the very start of the content", () => {
      const { container } = render(
        <MarkdownRenderer
          content="@alice thanks for this"
          mentionedUsernames={["alice"]}
        />,
      );

      const link = container.querySelector("a.mention");
      expect(link?.getAttribute("href")).toBe("/user/alice");
    });

    it("keeps surrounding text intact around a mention", () => {
      const { container } = render(
        <MarkdownRenderer
          content="before @alice after"
          mentionedUsernames={["alice"]}
        />,
      );

      expect(container.textContent).toBe("before @alice after");
    });

    it("does not linkify a mention inside a code span", () => {
      const { container } = render(
        <MarkdownRenderer
          content="use `@alice` as a placeholder"
          mentionedUsernames={["alice"]}
        />,
      );

      expect(container.querySelector("a.mention")).not.toBeInTheDocument();
      expect(container.querySelector("code")?.textContent).toBe("@alice");
    });

    it("does not linkify a mention immediately after markdown syntax with no real boundary character", () => {
      // The backend's mention.Extract runs once over the raw body: in
      // "Also *cool*@alice again", the character before "@" is "*", not
      // whitespace/paren/start, so the backend never extracts this second
      // occurrence as a mention. A per-mdast-text-node regex must not treat
      // the start of a later sibling node as a fresh "^" — that would
      // linkify text the backend-side extraction would never have resolved.
      const { container } = render(
        <MarkdownRenderer
          content="@alice thanks. Also *cool*@alice again"
          mentionedUsernames={["alice"]}
        />,
      );

      const links = container.querySelectorAll("a.mention");
      // Only the first, genuinely whitespace/start-preceded occurrence links.
      expect(links).toHaveLength(1);
      expect(links[0].textContent).toBe("@alice");
      expect(container.querySelector("em")?.textContent).toBe("cool");
      // The second occurrence stays literal text, not a second link.
      expect(container.textContent).toContain("@alice again");
    });

    it("still links a mention that legitimately follows markdown syntax with a real space", () => {
      const { container } = render(
        <MarkdownRenderer
          content="**bold** @alice"
          mentionedUsernames={["alice"]}
        />,
      );

      const link = container.querySelector("a.mention");
      expect(link).toBeInTheDocument();
      expect(link?.getAttribute("href")).toBe("/user/alice");
    });

    it("works in inline mode", () => {
      const { container } = render(
        <MarkdownRenderer
          content="hey @alice"
          mentionedUsernames={["alice"]}
          inline
        />,
      );

      expect(container.querySelector("a.mention")).toBeInTheDocument();
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
