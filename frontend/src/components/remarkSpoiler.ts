import type { Nodes, Parent, PhrasingContent, Text } from "mdast";

const DELIMITER = "!!";
const SUMMARY_LABEL = "Spoiler";

/**
 * A `!!` delimiter run found inside the inline children of a node, plus the
 * flanking information needed to decide whether it can open or close a spoiler.
 */
interface DelimiterToken {
  kind: "delimiter";
  canOpen: boolean;
  canClose: boolean;
}

interface NodeToken {
  kind: "node";
  node: PhrasingContent;
}

type Token = DelimiterToken | NodeToken;

function text(value: string): Text {
  return { type: "text", value };
}

/**
 * Builds the mdast node for a spoiler. It has no matching mdast type, so it
 * relies on `data.hName`, which `mdast-util-to-hast` applies when it converts
 * unknown nodes — producing `<details><summary>Spoiler</summary>…</details>`.
 * Both tags are already permitted by the renderer's sanitize schema.
 */
function spoiler(children: PhrasingContent[]): PhrasingContent {
  const node = {
    type: "spoiler",
    data: { hName: "details" },
    children: [
      {
        type: "spoilerSummary",
        data: { hName: "summary" },
        children: [text(SUMMARY_LABEL)],
      },
      ...children,
    ],
  };
  // `spoiler` is not part of the mdast content model; the cast keeps the tree
  // usable by remark while still producing valid HTML downstream.
  return node as unknown as PhrasingContent;
}

function isWhitespace(char: string | undefined): boolean {
  return char === undefined || /\s/.test(char);
}

/**
 * Splits the inline children of a node into nodes and `!!` delimiter runs.
 * Non-text nodes (emphasis, inline code, links, …) are kept opaque, so
 * `!!**bold** spoiler!!` still matches across sibling nodes and `!!` inside a
 * code span is never treated as a delimiter.
 */
function tokenize(children: PhrasingContent[]): Token[] {
  const tokens: Token[] = [];

  children.forEach((child, index) => {
    if (child.type !== "text") {
      tokens.push({ kind: "node", node: child });
      return;
    }

    const { value } = child;
    // A sibling node on either side counts as a non-whitespace neighbour.
    const neighbourBefore = index > 0 ? "x" : undefined;
    const neighbourAfter = index < children.length - 1 ? "x" : undefined;

    let cursor = 0;
    let at = value.indexOf(DELIMITER);

    while (at !== -1) {
      const before = value.slice(cursor, at);
      if (before) tokens.push({ kind: "node", node: text(before) });

      const previousChar = at > 0 ? value[at - 1] : neighbourBefore;
      const nextChar =
        at + DELIMITER.length < value.length
          ? value[at + DELIMITER.length]
          : neighbourAfter;

      tokens.push({
        kind: "delimiter",
        // Mirrors CommonMark's flanking rules: `Wow!! Amazing!!` must not turn
        // into a spoiler, but `!!he dies!!` must.
        canOpen: !isWhitespace(nextChar),
        canClose: !isWhitespace(previousChar),
      });

      cursor = at + DELIMITER.length;
      at = value.indexOf(DELIMITER, cursor);
    }

    const rest = value.slice(cursor);
    if (rest) tokens.push({ kind: "node", node: text(rest) });
  });

  return tokens;
}

/** Turns matched `!!…!!` pairs into spoiler nodes; leaves stray `!!` as text. */
function parse(tokens: Token[]): PhrasingContent[] {
  const result: PhrasingContent[] = [];
  let index = 0;

  while (index < tokens.length) {
    const token = tokens[index];

    if (token.kind === "node") {
      result.push(token.node);
      index += 1;
      continue;
    }

    if (!token.canOpen) {
      result.push(text(DELIMITER));
      index += 1;
      continue;
    }

    // Closing delimiter must be able to close and enclose at least one node.
    let close = -1;
    for (let j = index + 2; j < tokens.length; j += 1) {
      const candidate = tokens[j];
      if (candidate.kind === "delimiter" && candidate.canClose) {
        close = j;
        break;
      }
    }

    if (close === -1) {
      result.push(text(DELIMITER));
      index += 1;
      continue;
    }

    // Delimiters that could not close (e.g. `!! `) stay literal inside.
    const contents = tokens
      .slice(index + 1, close)
      .map((token) => (token.kind === "node" ? token.node : text(DELIMITER)));

    result.push(spoiler(contents));
    index = close + 1;
  }

  return result;
}

function hasChildren(node: Nodes): node is Nodes & Parent {
  return "children" in node && Array.isArray((node as Parent).children);
}

function transform(node: Nodes): void {
  if (!hasChildren(node)) return;

  // Never rewrite inside a link. `!!` in a link label is almost always part of
  // the text — a torrent called `Show !!secret!! 01` is the case that prompted
  // this — and rewriting it there splits one `<a>` into three with a block-level
  // `<details>` wedged between them, which turns a one-line chat row into a
  // disclosure widget.
  //
  // Escaping cannot solve it from the caller's side: this plugin runs on the
  // mdast after escapes have already resolved, so the backslashes are gone by
  // the time we see the text.
  //
  // A spoiler whose reveal is a link label is not a meaningful construct anyway.
  // A link *inside* a spoiler still works: the link is an opaque node to the
  // tokenizer, so `!!see [here](url)!!` is unaffected by this guard.
  if (node.type === "link" || node.type === "linkReference") return;

  // Depth first: inner containers (emphasis, links, …) are resolved before the
  // paragraph that holds them.
  node.children.forEach((child) => transform(child));

  const hasDelimiter = node.children.some(
    (child) => child.type === "text" && child.value.includes(DELIMITER),
  );
  if (!hasDelimiter) return;

  node.children = parse(
    tokenize(node.children as PhrasingContent[]),
  ) as typeof node.children;
}

/**
 * Remark plugin rendering `!!spoiler text!!` as a click-to-reveal
 * `<details>/<summary>` element.
 */
export function remarkSpoiler() {
  return (tree: Nodes): void => transform(tree);
}
