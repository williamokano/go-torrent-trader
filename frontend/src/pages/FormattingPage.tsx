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
        syntax: "**bold text**",
        preview: <b>bold text</b>,
      },
      {
        name: "Italic",
        syntax: "*italic text*",
        preview: <i>italic text</i>,
      },
      {
        name: "Strikethrough",
        syntax: "~~struck text~~",
        preview: <s>struck text</s>,
      },
      {
        name: "Bold & Italic",
        syntax: "***bold italic***",
        preview: (
          <b>
            <i>bold italic</i>
          </b>
        ),
      },
    ],
  },
  {
    title: "Headings",
    examples: [
      {
        name: "Heading 1",
        syntax: "# Heading 1",
        preview: <span style={{ fontSize: "1.5em", fontWeight: 700 }}>Heading 1</span>,
      },
      {
        name: "Heading 2",
        syntax: "## Heading 2",
        preview: <span style={{ fontSize: "1.3em", fontWeight: 700 }}>Heading 2</span>,
      },
      {
        name: "Heading 3",
        syntax: "### Heading 3",
        preview: <span style={{ fontSize: "1.1em", fontWeight: 700 }}>Heading 3</span>,
      },
    ],
  },
  {
    title: "Links & Images",
    examples: [
      {
        name: "Link",
        syntax: "[Click here](https://example.com)",
        preview: (
          <a href="#" onClick={(e) => e.preventDefault()}>
            Click here
          </a>
        ),
      },
      {
        name: "Auto-link",
        syntax: "https://example.com",
        preview: (
          <a href="#" onClick={(e) => e.preventDefault()}>
            https://example.com
          </a>
        ),
      },
      {
        name: "Image",
        syntax: "![alt text](https://example.com/image.png)",
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
        syntax: "`inline code`",
        preview: <code>inline code</code>,
      },
      {
        name: "Code block",
        syntax: "```\ncode block\nmultiple lines\n```",
        preview: (
          <pre className="formatting__preview-code">
            {"code block\nmultiple lines"}
          </pre>
        ),
      },
      {
        name: "Quote",
        syntax: "> quoted text",
        preview: <blockquote>quoted text</blockquote>,
      },
      {
        name: "Nested quote",
        syntax: "> first level\n>> nested quote",
        preview: (
          <blockquote>
            first level
            <blockquote>nested quote</blockquote>
          </blockquote>
        ),
      },
    ],
  },
  {
    title: "Lists",
    examples: [
      {
        name: "Unordered list",
        syntax: "- Item one\n- Item two\n- Item three",
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
        syntax: "1. First item\n2. Second item\n3. Third item",
        preview: (
          <ol className="formatting__preview-list">
            <li>First item</li>
            <li>Second item</li>
            <li>Third item</li>
          </ol>
        ),
      },
      {
        name: "Task list",
        syntax: "- [x] Completed task\n- [ ] Pending task",
        preview: (
          <ul className="formatting__preview-list" style={{ listStyle: "none" }}>
            <li>&#9745; Completed task</li>
            <li>&#9744; Pending task</li>
          </ul>
        ),
      },
    ],
  },
  {
    title: "Tables",
    examples: [
      {
        name: "Table",
        syntax:
          "| Header 1 | Header 2 |\n| -------- | -------- |\n| Cell 1   | Cell 2   |",
        preview: (
          <table className="formatting__preview-table">
            <thead>
              <tr>
                <th>Header 1</th>
                <th>Header 2</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>Cell 1</td>
                <td>Cell 2</td>
              </tr>
            </tbody>
          </table>
        ),
      },
    ],
  },
  {
    title: "Other",
    examples: [
      {
        name: "Horizontal rule",
        syntax: "---",
        preview: <hr className="formatting__preview-hr" />,
      },
      {
        name: "Spoiler",
        syntax: "!!hidden text!!",
        preview: (
          <details>
            <summary>Spoiler</summary>
            hidden text
          </details>
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
        Use Markdown to format text in descriptions, comments, and messages.
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
        This is a reference guide for supported Markdown formatting. Syntax can
        be combined (e.g.,{" "}
        <code className="formatting__syntax">***bold italic***</code>). Not all
        formatting may be available in all areas of the site.
      </div>
    </div>
  );
}
