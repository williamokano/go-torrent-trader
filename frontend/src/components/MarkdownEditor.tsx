import { useCallback, useEffect, useId, useRef, useState } from "react";
import type { KeyboardEvent as ReactKeyboardEvent } from "react";
import { Link } from "react-router-dom";
import { getConfig } from "@/config";
import { getAccessToken } from "@/features/auth/token";
import { MarkdownRenderer } from "./MarkdownRenderer";
import "./markdown-editor.css";

interface MarkdownEditorProps {
  value: string;
  onChange: (value: string) => void;
  label?: string;
  id?: string;
  placeholder?: string;
  rows?: number;
  className?: string;
}

// Note: no `required` prop on purpose. The textarea is unmounted while the
// preview is open, so native form validation could be bypassed by submitting
// from preview mode. Callers guard emptiness on their submit button instead.

/** Wraps the selection (`**bold**`) or prefixes each selected line (`> quote`). */
type ToolbarAction =
  | { kind: "wrap"; before: string; after: string; sample: string }
  | { kind: "prefix"; prefix: string; sample: string };

interface ToolbarButton {
  key: string;
  label: string;
  title: string;
  action: ToolbarAction;
}

const TOOLBAR: ToolbarButton[] = [
  {
    key: "bold",
    label: "B",
    title: "Bold",
    action: { kind: "wrap", before: "**", after: "**", sample: "bold text" },
  },
  {
    key: "italic",
    label: "I",
    title: "Italic",
    action: { kind: "wrap", before: "*", after: "*", sample: "italic text" },
  },
  {
    key: "link",
    label: "Link",
    title: "Link",
    action: {
      kind: "wrap",
      before: "[",
      after: "](https://)",
      sample: "link text",
    },
  },
  {
    key: "image",
    label: "Image",
    title: "Image",
    action: {
      kind: "wrap",
      before: "![",
      after: "](https://)",
      sample: "alt text",
    },
  },
  {
    key: "code",
    label: "Code",
    title: "Code",
    action: { kind: "wrap", before: "`", after: "`", sample: "code" },
  },
  {
    key: "quote",
    label: "Quote",
    title: "Blockquote",
    action: { kind: "prefix", prefix: "> ", sample: "quoted text" },
  },
  {
    key: "spoiler",
    label: "Spoiler",
    title: "Spoiler (click to reveal)",
    action: { kind: "wrap", before: "!!", after: "!!", sample: "spoiler" },
  },
  {
    key: "list",
    label: "List",
    title: "Bulleted list",
    action: { kind: "prefix", prefix: "- ", sample: "list item" },
  },
];

interface UserSuggestion {
  id: number;
  username: string;
}

// Mirrors the backend mention parser (`(?:^|[\s(])@(\w+)` in
// internal/listener/notification.go) so a suggestion we insert is a mention
// that actually notifies. `\w*` (not `\w+`) lets it match the bare `@` the
// instant it is typed. Anchored to the caret via `$`.
const MENTION_RE = /(?:^|[\s(])@(\w*)$/;

/** The active `@mention` token immediately before the caret, if any. */
function detectMention(text: string, caret: number) {
  const match = text.slice(0, caret).match(MENTION_RE);
  if (!match) return null;
  const query = match[1];
  return { query, at: caret - query.length - 1 };
}

/**
 * Lightweight Markdown editor: a plain textarea with a toolbar that inserts
 * Markdown syntax at the cursor, plus a preview rendered by MarkdownRenderer.
 * Deliberately free of editor frameworks.
 */
export function MarkdownEditor({
  value,
  onChange,
  label,
  id,
  placeholder,
  rows = 6,
  className = "",
}: MarkdownEditorProps) {
  const generatedId = useId();
  const textareaId = id ?? generatedId;
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const [preview, setPreview] = useState(false);

  // Selection to restore after a toolbar insert. The textarea is controlled, so
  // the caret can only be moved once the new value has been rendered.
  const pendingSelection = useRef<[number, number] | null>(null);

  useEffect(() => {
    const selection = pendingSelection.current;
    const textarea = textareaRef.current;
    if (!selection || !textarea) return;

    pendingSelection.current = null;
    textarea.focus();
    textarea.setSelectionRange(selection[0], selection[1]);
  });

  // --- @mention autocomplete ---------------------------------------------
  const [mentions, setMentions] = useState<UserSuggestion[]>([]);
  const [mentionOpen, setMentionOpen] = useState(false);
  const [activeIndex, setActiveIndex] = useState(0);
  const mentionStart = useRef<number | null>(null); // index of the `@`
  const debounceRef = useRef<ReturnType<typeof setTimeout>>(undefined);
  // Bumped on every search and on close; a resolving fetch whose seq is stale
  // is dropped, so out-of-order responses can't repopulate a closed dropdown.
  const reqSeq = useRef(0);

  useEffect(() => () => clearTimeout(debounceRef.current), []);

  const closeMentions = useCallback(() => {
    reqSeq.current++;
    clearTimeout(debounceRef.current);
    mentionStart.current = null;
    setMentionOpen(false);
    setMentions([]);
  }, []);

  const searchUsers = useCallback((query: string) => {
    clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(async () => {
      const seq = ++reqSeq.current;
      try {
        const token = getAccessToken();
        const res = await fetch(
          `${getConfig().API_URL}/api/v1/users?search=${encodeURIComponent(query)}&per_page=8`,
          { headers: token ? { Authorization: `Bearer ${token}` } : {} },
        );
        if (!res.ok || seq !== reqSeq.current) return;
        const data = await res.json();
        if (seq !== reqSeq.current) return;
        const users: UserSuggestion[] = (data?.users ?? []).map(
          (u: UserSuggestion) => ({ id: u.id, username: u.username }),
        );
        setMentions(users);
        setActiveIndex(0);
        setMentionOpen(users.length > 0);
      } catch {
        // Autocomplete is best-effort; a failed lookup just shows nothing.
      }
    }, 250);
  }, []);

  const updateMention = useCallback(
    (text: string, caret: number | null) => {
      const mention = detectMention(text, caret ?? text.length);
      // Require at least one character after `@` so a bare `@` doesn't dump the
      // whole member list (the search endpoint ignores an empty `search`).
      if (!mention || mention.query.length < 1) {
        closeMentions();
        return;
      }
      mentionStart.current = mention.at;
      searchUsers(mention.query);
    },
    [closeMentions, searchUsers],
  );

  function applyMention(username: string) {
    const textarea = textareaRef.current;
    const at = mentionStart.current;
    if (!textarea || at === null) return;

    const insert = `@${username} `;
    const next =
      value.slice(0, at) + insert + value.slice(textarea.selectionStart);
    const caret = at + insert.length;

    pendingSelection.current = [caret, caret];
    onChange(next);
    closeMentions();
  }

  function handleMentionKeyDown(e: ReactKeyboardEvent<HTMLTextAreaElement>) {
    if (!mentionOpen || mentions.length === 0) return;
    switch (e.key) {
      case "ArrowDown":
        e.preventDefault();
        setActiveIndex((i) => (i + 1) % mentions.length);
        break;
      case "ArrowUp":
        e.preventDefault();
        setActiveIndex((i) => (i - 1 + mentions.length) % mentions.length);
        break;
      case "Enter":
      case "Tab":
        e.preventDefault();
        applyMention(mentions[activeIndex].username);
        break;
      case "Escape":
        e.preventDefault();
        closeMentions();
        break;
    }
  }

  function apply(action: ToolbarAction) {
    const textarea = textareaRef.current;
    if (!textarea) return;

    const { selectionStart, selectionEnd } = textarea;

    if (action.kind === "wrap") {
      const selected =
        value.slice(selectionStart, selectionEnd) || action.sample;
      const next =
        value.slice(0, selectionStart) +
        action.before +
        selected +
        action.after +
        value.slice(selectionEnd);

      const start = selectionStart + action.before.length;
      pendingSelection.current = [start, start + selected.length];
      onChange(next);
      return;
    }

    // Prefix every line the selection touches.
    const lineStart = value.lastIndexOf("\n", selectionStart - 1) + 1;
    const newlineAfter = value.indexOf("\n", selectionEnd);
    const lineEnd = newlineAfter === -1 ? value.length : newlineAfter;

    const block = value.slice(lineStart, lineEnd) || action.sample;
    const prefixed = block
      .split("\n")
      .map((line) => action.prefix + line)
      .join("\n");

    const next = value.slice(0, lineStart) + prefixed + value.slice(lineEnd);

    pendingSelection.current = [
      lineStart + action.prefix.length,
      lineStart + prefixed.length,
    ];
    onChange(next);
  }

  return (
    <div className={`markdown-editor ${className}`.trim()}>
      <div className="markdown-editor__header">
        {label && (
          <label className="markdown-editor__label" htmlFor={textareaId}>
            {label}
          </label>
        )}
        <div className="markdown-editor__tabs">
          <button
            type="button"
            className={`markdown-editor__tab${preview ? "" : " markdown-editor__tab--active"}`}
            onClick={() => setPreview(false)}
            aria-pressed={!preview}
          >
            Write
          </button>
          <button
            type="button"
            className={`markdown-editor__tab${preview ? " markdown-editor__tab--active" : ""}`}
            onClick={() => setPreview(true)}
            aria-pressed={preview}
          >
            Preview
          </button>
        </div>
      </div>

      {!preview && (
        <div
          className="markdown-editor__toolbar"
          role="toolbar"
          aria-label="Formatting"
        >
          {TOOLBAR.map((button) => (
            <button
              key={button.key}
              type="button"
              className={`markdown-editor__btn markdown-editor__btn--${button.key}`}
              title={button.title}
              aria-label={button.title}
              onClick={() => apply(button.action)}
            >
              {button.label}
            </button>
          ))}
        </div>
      )}

      {preview ? (
        <div
          className="markdown-editor__preview"
          data-testid="markdown-preview"
        >
          {value.trim() ? (
            <MarkdownRenderer content={value} />
          ) : (
            <p className="markdown-editor__preview-empty">
              Nothing to preview.
            </p>
          )}
        </div>
      ) : (
        <div className="markdown-editor__input">
          <textarea
            id={textareaId}
            ref={textareaRef}
            className="markdown-editor__textarea"
            value={value}
            onChange={(e) => {
              onChange(e.target.value);
              updateMention(e.target.value, e.target.selectionStart);
            }}
            onSelect={(e) => {
              // Close if the caret moved out of the active mention token; never
              // re-search on a plain caret move, only on typing.
              if (
                mentionOpen &&
                !detectMention(
                  e.currentTarget.value,
                  e.currentTarget.selectionStart,
                )
              ) {
                closeMentions();
              }
            }}
            onKeyDown={handleMentionKeyDown}
            onBlur={closeMentions}
            rows={rows}
            placeholder={placeholder}
          />
          {mentionOpen && mentions.length > 0 && (
            <ul
              className="markdown-editor__mentions"
              role="listbox"
              aria-label="User suggestions"
            >
              {mentions.map((user, i) => (
                <li
                  key={user.id}
                  role="option"
                  aria-selected={i === activeIndex}
                >
                  <button
                    type="button"
                    className={`markdown-editor__mention${i === activeIndex ? " markdown-editor__mention--active" : ""}`}
                    // preventDefault keeps focus (and the caret) in the textarea
                    // so the blur handler doesn't close the list before we insert.
                    onMouseDown={(e) => {
                      e.preventDefault();
                      applyMention(user.username);
                    }}
                    onMouseEnter={() => setActiveIndex(i)}
                  >
                    @{user.username}
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>
      )}

      <p className="markdown-editor__hint">
        Markdown supported — <Link to="/formatting">formatting reference</Link>
      </p>
    </div>
  );
}
