import { memo } from "react";
import ReactMarkdown, { type Components } from "react-markdown";
import remarkGfm from "remark-gfm";
import rehypeRaw from "rehype-raw";
import rehypeSanitize, { defaultSchema } from "rehype-sanitize";
import { remarkSpoiler } from "./remarkSpoiler";
import { remarkMention } from "./remarkMention";
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
  /**
   * Usernames resolved as real @mentions when this content was saved (see
   * ResolveMentionedUsernames on the backend) — only these tokens render as
   * links to the mentioned user's profile; anything else stays plain text.
   */
  mentionedUsernames?: string[];
}

// Extend the default sanitize schema to allow <details>/<summary> for
// spoilers, and a "mention" class on <a> so a mention link can be styled
// distinctly from a plain link. defaultSchema.attributes.a already has one
// `["className", "data-footnote-backref"]` entry — hast-util-sanitize's
// findDefinition takes the *first* entry matching a given property name, so
// a second separate `["className", ...]` tuple would silently be dead code.
// "mention" has to be appended to that existing entry's allow-list instead.
const sanitizeSchema = {
  ...defaultSchema,
  tagNames: [...(defaultSchema.tagNames ?? []), "details", "summary"],
  attributes: {
    ...defaultSchema.attributes,
    a: (defaultSchema.attributes?.a ?? []).map((entry) =>
      Array.isArray(entry) && entry[0] === "className"
        ? [...entry, "mention"]
        : entry,
    ),
  },
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
  mentionedUsernames,
}: MarkdownRendererProps) {
  const classes = [
    "markdown-body",
    inline && "markdown-body--inline",
    className,
  ]
    .filter(Boolean)
    .join(" ");

  const Wrapper = inline ? "span" : "div";
  const validMentions = new Set(mentionedUsernames ?? []);

  return (
    <Wrapper className={classes}>
      <ReactMarkdown
        remarkPlugins={[
          remarkGfm,
          remarkSpoiler,
          [remarkMention, { validMentions }],
        ]}
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
