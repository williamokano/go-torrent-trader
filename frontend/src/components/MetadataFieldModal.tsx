import { useState } from "react";
import { Modal } from "@/components/modal/Modal";
import { Input, Select, Checkbox } from "@/components/form";
import type { MetadataField, MetadataFieldType } from "@/utils/metadata";
import "./metadata-field-modal.css";

const TYPE_OPTIONS: { value: MetadataFieldType; label: string }[] = [
  { value: "text", label: "Text" },
  { value: "number", label: "Number" },
  { value: "select", label: "Select (one)" },
  { value: "multiselect", label: "Multi-select" },
  { value: "boolean", label: "Checkbox" },
];

// Mirrors the backend key rule (internal/metadata): a lowercase identifier.
const KEY_RE = /^[a-z][a-z0-9_]*$/;

function numOrUndef(s: string): number | undefined {
  if (s.trim() === "") return undefined;
  const n = Number(s);
  return Number.isNaN(n) ? undefined : n;
}

function parseOptions(s: string): string[] {
  return s
    .split(",")
    .map((x) => x.trim())
    .filter((x) => x !== "");
}

interface MetadataFieldModalProps {
  /** The field being edited, or null to add a new one. */
  initial: MetadataField | null;
  /** Keys already used by the other fields, for duplicate detection. */
  existingKeys: string[];
  onSave: (field: MetadataField) => void;
  onClose: () => void;
}

/**
 * Modal form for a single metadata field definition. Owns a local draft so the
 * category schema is only mutated when the admin clicks Save Field. The parent
 * mounts it only while open, so the draft always starts fresh from `initial`.
 */
export function MetadataFieldModal({
  initial,
  existingKeys,
  onSave,
  onClose,
}: MetadataFieldModalProps) {
  const isEdit = initial != null;
  const [draft, setDraft] = useState<MetadataField>(
    () => initial ?? { key: "", label: "", type: "text" },
  );
  // Options are edited as raw text so commas type naturally; parsed on save.
  const [optionsText, setOptionsText] = useState<string>(() =>
    (initial?.options ?? []).join(", "),
  );
  const [error, setError] = useState<string | null>(null);

  const patch = (p: Partial<MetadataField>) =>
    setDraft((d) => ({ ...d, ...p }));

  const handleSave = () => {
    const key = draft.key.trim();
    const label = draft.label.trim();

    if (!KEY_RE.test(key)) {
      setError(
        "Key must start with a lowercase letter and use only lowercase letters, numbers, and underscores.",
      );
      return;
    }
    if (existingKeys.includes(key)) {
      setError(`A field with key "${key}" already exists.`);
      return;
    }
    if (!label) {
      setError("Label is required.");
      return;
    }

    const field: MetadataField = { key, label, type: draft.type };
    if (draft.required) field.required = true;

    switch (draft.type) {
      case "select":
      case "multiselect": {
        const options = parseOptions(optionsText);
        if (options.length === 0) {
          setError("Add at least one option.");
          return;
        }
        field.options = options;
        if (draft.type === "multiselect" && draft.max_items != null) {
          field.max_items = draft.max_items;
        }
        break;
      }
      case "number": {
        if (draft.min != null && draft.max != null && draft.min > draft.max) {
          setError("Min must be less than or equal to Max.");
          return;
        }
        if (draft.min != null) field.min = draft.min;
        if (draft.max != null) field.max = draft.max;
        if (draft.integer) field.integer = true;
        break;
      }
      case "text": {
        if (draft.max_length != null) field.max_length = draft.max_length;
        if (draft.pattern?.trim()) field.pattern = draft.pattern.trim();
        break;
      }
    }

    onSave(field);
  };

  return (
    <Modal
      isOpen
      onClose={onClose}
      title={isEdit ? "Edit Field" : "Add Field"}
      closeOnEscape={false}
      closeOnDismissClick={false}
    >
      <div className="field-modal">
        {error && <p className="field-modal__error">{error}</p>}

        <div className="field-modal__grid">
          <Input
            label="Key"
            value={draft.key}
            placeholder="e.g. year"
            onChange={(e) => patch({ key: e.target.value })}
          />
          <Input
            label="Label"
            value={draft.label}
            placeholder="e.g. Year"
            onChange={(e) => patch({ label: e.target.value })}
          />
          <Select
            label="Type"
            options={TYPE_OPTIONS}
            value={draft.type}
            onChange={(e) =>
              patch({ type: e.target.value as MetadataFieldType })
            }
          />
        </div>

        {(draft.type === "select" || draft.type === "multiselect") && (
          <div className="field-modal__grid">
            <Input
              label="Options (comma separated)"
              value={optionsText}
              placeholder="x264, x265, AV1"
              onChange={(e) => setOptionsText(e.target.value)}
            />
            {draft.type === "multiselect" && (
              <Input
                label="Max items"
                type="number"
                value={draft.max_items != null ? String(draft.max_items) : ""}
                onChange={(e) =>
                  patch({ max_items: numOrUndef(e.target.value) })
                }
              />
            )}
          </div>
        )}

        {draft.type === "number" && (
          <div className="field-modal__grid">
            <Input
              label="Min"
              type="number"
              value={draft.min != null ? String(draft.min) : ""}
              onChange={(e) => patch({ min: numOrUndef(e.target.value) })}
            />
            <Input
              label="Max"
              type="number"
              value={draft.max != null ? String(draft.max) : ""}
              onChange={(e) => patch({ max: numOrUndef(e.target.value) })}
            />
            <Checkbox
              label="Whole numbers only"
              checked={Boolean(draft.integer)}
              onChange={(e) => patch({ integer: e.target.checked })}
            />
          </div>
        )}

        {draft.type === "text" && (
          <div className="field-modal__grid">
            <Input
              label="Max length"
              type="number"
              value={draft.max_length != null ? String(draft.max_length) : ""}
              onChange={(e) =>
                patch({ max_length: numOrUndef(e.target.value) })
              }
            />
            <Input
              label="Pattern (regex)"
              value={draft.pattern ?? ""}
              placeholder="optional"
              onChange={(e) => patch({ pattern: e.target.value || undefined })}
            />
          </div>
        )}

        <Checkbox
          label="Required"
          checked={Boolean(draft.required)}
          onChange={(e) => patch({ required: e.target.checked })}
        />

        <div className="field-modal__actions">
          <button className="admin-btn admin-btn--ghost" onClick={onClose}>
            Cancel
          </button>
          <button
            className="admin-btn admin-btn--primary"
            onClick={handleSave}
            disabled={!draft.key.trim() || !draft.label.trim()}
          >
            Save Field
          </button>
        </div>
      </div>
    </Modal>
  );
}
