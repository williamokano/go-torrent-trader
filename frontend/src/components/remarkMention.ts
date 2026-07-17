import type { Nodes, Parent, PhrasingContent, Text } from "mdast";

// Mirrors the backend's mention.Extract regex (internal/mention/mention.go):
// same left-boundary rule (start of string, whitespace, or an opening paren)
// — so a token this plugin ever links is a token the backend could also have
// extracted. Without matching that boundary, an email-shaped "foo@bar" would
// get linkified whenever "bar" happens to be a genuine mention elsewhere in
// the same content.
//
// The backend runs its regex once over the whole raw body; this plugin runs
// per mdast text node, and inline Markdown (emphasis, links, code spans)
// splits one paragraph into several sibling text nodes. If every node's `^`
// meant "true start of paragraph," a mention immediately after such syntax —
// "*cool*@alice", where the backend sees "@" preceded by "*", not
// whitespace/paren/start, and never extracts it — would wrongly linkify here
// on the frontend, since mdast's node boundary looks like a string start.
// MENTION_RE (with `^`) is only used for a parent's first child; every later
// sibling uses MENTION_RE_REST, which drops the `^` alternative so a match
// can only start where an actual whitespace/paren character exists in that
// node's own text — never at the bare start of a non-first node.
const MENTION_RE = /(?:^|[\s(])@(\w+)/g;
const MENTION_RE_REST = /[\s(]@(\w+)/g;

function text(value: string): Text {
  return { type: "text", value };
}

/** A link to the mentioned user's profile, styled distinctly from a plain link. */
function mentionLink(username: string): PhrasingContent {
  return {
    type: "link",
    url: `/user/${username}`,
    // hast represents className as an array, not a bare string — passing a
    // string here renders (mdast-util-to-hast doesn't validate the shape)
    // but hast-util-sanitize's allowlist match against defaultSchema's
    // ['className', 'mention'] tuple only recognizes the array form, so a
    // bare string silently gets stripped.
    data: { hProperties: { className: ["mention"] } },
    children: [text(`@${username}`)],
  } as PhrasingContent;
}

/**
 * Splits one text node into a mix of plain text and mention links. A mention
 * is always fully self-contained within a single text run — it can't span a
 * bold/link boundary, the same limitation the backend's own regex has, since
 * neither understands Markdown structure. Only tokens present in
 * `validMentions` (resolved once at write time) are linkified; everything
 * else — typos, non-mention `@` uses — stays literal text.
 */
function linkifyText(
  node: Text,
  validMentions: Set<string>,
  isFirstChild: boolean,
): PhrasingContent[] {
  const value = node.value;
  const re = isFirstChild ? MENTION_RE : MENTION_RE_REST;
  re.lastIndex = 0;

  const out: PhrasingContent[] = [];
  let cursor = 0;
  let matched = false;
  let match: RegExpExecArray | null;

  while ((match = re.exec(value)) !== null) {
    const username = match[1];
    if (!validMentions.has(username)) continue;
    matched = true;

    // match[0] includes the boundary character (whitespace/paren), if any;
    // `atIndex` is the position of the `@` itself, so the boundary char is
    // left in the preceding plain-text slice rather than swallowed.
    const atIndex = match.index + match[0].length - (1 + username.length);
    if (atIndex > cursor) {
      out.push(text(value.slice(cursor, atIndex)));
    }
    out.push(mentionLink(username));
    cursor = atIndex + 1 + username.length;
  }

  if (!matched) return [node];
  if (cursor < value.length) {
    out.push(text(value.slice(cursor)));
  }
  return out;
}

function hasChildren(node: Nodes): node is Nodes & Parent {
  return "children" in node && Array.isArray((node as Parent).children);
}

function transform(node: Nodes, validMentions: Set<string>): void {
  if (!hasChildren(node)) return;

  // Depth first: inner containers (emphasis, links, …) are resolved before
  // the paragraph that holds them.
  node.children.forEach((child) => transform(child, validMentions));

  const next: PhrasingContent[] = [];
  let changed = false;
  const children = node.children as PhrasingContent[];
  children.forEach((child, index) => {
    if (child.type === "text" && child.value.includes("@")) {
      const linked = linkifyText(child, validMentions, index === 0);
      if (linked.length !== 1 || linked[0] !== child) changed = true;
      next.push(...linked);
    } else {
      next.push(child);
    }
  });
  if (changed) {
    node.children = next as typeof node.children;
  }
}

export interface RemarkMentionOptions {
  /** Usernames resolved as real mentions when the content was saved. */
  validMentions: Set<string>;
}

/**
 * Remark plugin turning a resolved `@username` mention into a link to that
 * user's profile. "Resolved" means the set of valid usernames was computed
 * once, server-side, at write time (see ResolveMentionedUsernames on the
 * backend) — this plugin never itself decides whether a mention is real, it
 * only renders the ones already known to be.
 */
export function remarkMention({ validMentions }: RemarkMentionOptions) {
  return (tree: Nodes): void => {
    if (validMentions.size === 0) return;
    transform(tree, validMentions);
  };
}
