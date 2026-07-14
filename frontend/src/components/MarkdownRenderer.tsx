import { memo } from "react";
import ReactMarkdown, { type Components } from "react-markdown";
import remarkGfm from "remark-gfm";
import rehypeRaw from "rehype-raw";
import rehypeSanitize, { defaultSchema } from "rehype-sanitize";
import { remarkSpoiler } from "./remarkSpoiler";
import "./markdown.css";

interface MarkdownRendererProps {
  content: string;
  className?: string;
  /**
   * Renders inside a `<span>` and drops block-level elements (headings,
   * tables, images, …). Used for one-line surfaces such as chat, where a
   * heading or a table would wreck the layout.
   */
  inline?: boolean;
}

// Extend the default sanitize schema to allow <details>/<summary> for spoilers
const sanitizeSchema = {
  ...defaultSchema,
  tagNames: [...(defaultSchema.tagNames ?? []), "details", "summary"],
};

// Elements kept in inline mode; anything else is unwrapped to its text.
const INLINE_ELEMENTS = [
  "a",
  "br",
  "code",
  "del",
  "details",
  "em",
  "strong",
  "summary",
];

const components: Components = {
  // <details> is block-level and may not nest inside <p>, so a paragraph that
  // holds a spoiler is rendered as a <div> instead.
  p({ node, children, ...props }) {
    const hasSpoiler = node?.children.some(
      (child) => child.type === "element" && child.tagName === "details",
    );

    return hasSpoiler ? (
      <div className="markdown-body__paragraph" {...props}>
        {children}
      </div>
    ) : (
      <p {...props}>{children}</p>
    );
  },
};

/**
 * Renders Markdown content safely. Supports GFM (tables, strikethrough,
 * task lists, autolinks), `!!spoilers!!`, and raw HTML (sanitized to
 * prevent XSS).
 *
 * Memoized: parsing is not free, and lists of rendered content (chat, comments,
 * forum posts) re-render whenever a single sibling changes.
 */
export const MarkdownRenderer = memo(function MarkdownRenderer({
  content,
  className = "",
  inline = false,
}: MarkdownRendererProps) {
  const classes = [
    "markdown-body",
    inline && "markdown-body--inline",
    className,
  ]
    .filter(Boolean)
    .join(" ");

  const Wrapper = inline ? "span" : "div";

  return (
    <Wrapper className={classes}>
      <ReactMarkdown
        remarkPlugins={[remarkGfm, remarkSpoiler]}
        rehypePlugins={[rehypeRaw, [rehypeSanitize, sanitizeSchema]]}
        components={components}
        allowedElements={inline ? INLINE_ELEMENTS : undefined}
        unwrapDisallowed={inline}
      >
        {content}
      </ReactMarkdown>
    </Wrapper>
  );
});
