import { useEffect, useId, useRef, useState } from "react";
import { Link } from "react-router-dom";
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
        <textarea
          id={textareaId}
          ref={textareaRef}
          className="markdown-editor__textarea"
          value={value}
          onChange={(e) => onChange(e.target.value)}
          rows={rows}
          placeholder={placeholder}
        />
      )}

      <p className="markdown-editor__hint">
        Markdown supported — <Link to="/formatting">formatting reference</Link>
      </p>
    </div>
  );
}
