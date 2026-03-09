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
          <span className="formatting__preview-muted">
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
          <pre className="formatting__preview-code">
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
        preview: (
          <span className="formatting__preview-color-red">red text</span>
        ),
      },
      {
        name: "Color (hex)",
        syntax: "[color=#00ff00]green text[/color]",
        preview: (
          <span className="formatting__preview-color-green">green text</span>
        ),
      },
      {
        name: "Size (small)",
        syntax: "[size=1]small text[/size]",
        preview: (
          <span className="formatting__preview-size-small">small text</span>
        ),
      },
      {
        name: "Size (large)",
        syntax: "[size=5]large text[/size]",
        preview: (
          <span className="formatting__preview-size-large">large text</span>
        ),
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
          <ul className="formatting__preview-list">
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
          <ol className="formatting__preview-list">
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
        preview: <hr className="formatting__preview-hr" />,
      },
      {
        name: "Spoiler",
        syntax: "[spoiler]hidden text[/spoiler]",
        preview: (
          <span
            className="formatting__preview-spoiler"
            title="Hover or click to reveal"
          >
            hidden text
          </span>
        ),
      },
      {
        name: "Align center",
        syntax: "[center]centered text[/center]",
        preview: (
          <div className="formatting__preview-center">centered text</div>
        ),
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
                <th scope="col">Format</th>
                <th scope="col">Syntax</th>
                <th scope="col">Preview</th>
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
