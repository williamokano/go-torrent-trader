import "./formatting.css";

interface FormatExample {
  name: string;
  syntax: string;
  preview: React.ReactNode;
}

interface FormatSection {
  title: string;
  examples: FormatExample[];
}

const FORMAT_SECTIONS: FormatSection[] = [
  {
    title: "Text Styling",
    examples: [
      {
        name: "Bold",
        syntax: "[b]bold text[/b]",
        preview: <b>bold text</b>,
      },
      {
        name: "Italic",
        syntax: "[i]italic text[/i]",
        preview: <i>italic text</i>,
      },
      {
        name: "Underline",
        syntax: "[u]underlined text[/u]",
        preview: <u>underlined text</u>,
      },
      {
        name: "Strikethrough",
        syntax: "[s]struck text[/s]",
        preview: <s>struck text</s>,
      },
    ],
  },
  {
    title: "Links & Images",
    examples: [
      {
        name: "URL (auto label)",
        syntax: "[url]https://example.com[/url]",
        preview: (
          <a href="#" onClick={(e) => e.preventDefault()}>
            https://example.com
          </a>
        ),
      },
      {
        name: "URL (custom label)",
        syntax: "[url=https://example.com]Click here[/url]",
        preview: (
          <a href="#" onClick={(e) => e.preventDefault()}>
            Click here
          </a>
        ),
      },
      {
        name: "Image",
        syntax: "[img]https://example.com/image.png[/img]",
        preview: (
          <span style={{ color: "var(--color-text-muted)" }}>
            (image would render here)
          </span>
        ),
      },
    ],
  },
  {
    title: "Code & Quotes",
    examples: [
      {
        name: "Inline code",
        syntax: "[code]inline code[/code]",
        preview: <code>inline code</code>,
      },
      {
        name: "Code block",
        syntax: "[pre]code block\nmultiple lines[/pre]",
        preview: (
          <pre
            style={{
              fontFamily: "var(--font-mono, monospace)",
              fontSize: "0.85em",
              background: "var(--color-bg-tertiary, #2a2a2a)",
              padding: "0.3rem 0.5rem",
              borderRadius: "3px",
            }}
          >
            {"code block\nmultiple lines"}
          </pre>
        ),
      },
      {
        name: "Quote",
        syntax: "[quote]quoted text[/quote]",
        preview: <blockquote>quoted text</blockquote>,
      },
      {
        name: "Named quote",
        syntax: "[quote=username]their words[/quote]",
        preview: (
          <blockquote>
            <strong>username</strong> wrote: their words
          </blockquote>
        ),
      },
    ],
  },
  {
    title: "Colors & Sizes",
    examples: [
      {
        name: "Color",
        syntax: "[color=red]red text[/color]",
        preview: <span style={{ color: "red" }}>red text</span>,
      },
      {
        name: "Color (hex)",
        syntax: "[color=#00ff00]green text[/color]",
        preview: <span style={{ color: "#00ff00" }}>green text</span>,
      },
      {
        name: "Size (small)",
        syntax: "[size=1]small text[/size]",
        preview: <span style={{ fontSize: "0.75em" }}>small text</span>,
      },
      {
        name: "Size (large)",
        syntax: "[size=5]large text[/size]",
        preview: <span style={{ fontSize: "1.5em" }}>large text</span>,
      },
    ],
  },
  {
    title: "Lists",
    examples: [
      {
        name: "Unordered list",
        syntax: "[list]\n[*]Item one\n[*]Item two\n[*]Item three\n[/list]",
        preview: (
          <ul style={{ paddingLeft: "1.5rem", margin: 0 }}>
            <li>Item one</li>
            <li>Item two</li>
            <li>Item three</li>
          </ul>
        ),
      },
      {
        name: "Ordered list",
        syntax:
          "[list=1]\n[*]First item\n[*]Second item\n[*]Third item\n[/list]",
        preview: (
          <ol style={{ paddingLeft: "1.5rem", margin: 0 }}>
            <li>First item</li>
            <li>Second item</li>
            <li>Third item</li>
          </ol>
        ),
      },
    ],
  },
  {
    title: "Other",
    examples: [
      {
        name: "Horizontal rule",
        syntax: "[hr]",
        preview: (
          <hr
            style={{
              border: "none",
              borderTop: "1px solid var(--color-border, #333)",
              margin: "0.25rem 0",
            }}
          />
        ),
      },
      {
        name: "Spoiler",
        syntax: "[spoiler]hidden text[/spoiler]",
        preview: (
          <span
            style={{
              background: "var(--color-text-primary)",
              color: "var(--color-text-primary)",
              padding: "0 0.25rem",
              borderRadius: "2px",
              cursor: "pointer",
            }}
            title="Hover or click to reveal"
          >
            hidden text
          </span>
        ),
      },
      {
        name: "Align center",
        syntax: "[center]centered text[/center]",
        preview: <div style={{ textAlign: "center" }}>centered text</div>,
      },
    ],
  },
];

export function FormattingPage() {
  return (
    <div className="formatting">
      <h1 className="formatting__title">Formatting Reference</h1>
      <p className="formatting__subtitle">
        Use BBCode tags to format text in descriptions, comments, and messages.
      </p>

      {FORMAT_SECTIONS.map((section) => (
        <section key={section.title} className="formatting__section">
          <h2 className="formatting__section-title">{section.title}</h2>
          <table className="formatting__table">
            <thead>
              <tr>
                <th>Format</th>
                <th>Syntax</th>
                <th>Preview</th>
              </tr>
            </thead>
            <tbody>
              {section.examples.map((example) => (
                <tr key={example.name}>
                  <td>{example.name}</td>
                  <td>
                    <span className="formatting__syntax">{example.syntax}</span>
                  </td>
                  <td className="formatting__preview">{example.preview}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </section>
      ))}

      <div className="formatting__note">
        This is a reference guide for supported formatting tags. Tags can be
        nested (e.g.,{" "}
        <code className="formatting__syntax">[b][i]bold italic[/i][/b]</code>).
        Not all tags may be available in all areas of the site.
      </div>
    </div>
  );
}
