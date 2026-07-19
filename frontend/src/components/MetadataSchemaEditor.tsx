import { Input, Select, Checkbox } from "@/components/form";
import type { MetadataField, MetadataFieldType } from "@/utils/metadata";
import "./metadata-schema-editor.css";

interface MetadataSchemaEditorProps {
  value: MetadataField[];
  onChange: (fields: MetadataField[]) => void;
}

const TYPE_OPTIONS: { value: MetadataFieldType; label: string }[] = [
  { value: "text", label: "Text" },
  { value: "number", label: "Number" },
  { value: "select", label: "Select (one)" },
  { value: "multiselect", label: "Multi-select" },
  { value: "boolean", label: "Checkbox" },
];

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

/**
 * Admin editor for a category's metadata field definitions. Emits the
 * `metadata_schema` array; the backend validates it on save.
 */
export function MetadataSchemaEditor({
  value,
  onChange,
}: MetadataSchemaEditorProps) {
  const patch = (index: number, p: Partial<MetadataField>) => {
    onChange(value.map((f, i) => (i === index ? { ...f, ...p } : f)));
  };

  const removeField = (index: number) => {
    onChange(value.filter((_, i) => i !== index));
  };

  const addField = () => {
    onChange([...value, { key: "", label: "", type: "text" }]);
  };

  return (
    <div className="schema-editor">
      <div className="schema-editor__header">
        <span className="form-label">Metadata Fields</span>
        <button
          type="button"
          className="admin-btn admin-btn--ghost admin-btn--sm"
          onClick={addField}
        >
          Add Field
        </button>
      </div>

      {value.length === 0 && (
        <p className="schema-editor__empty">
          No custom fields. Uploads in this category collect only the standard
          fields.
        </p>
      )}

      {value.map((field, index) => (
        <div
          key={index}
          className="schema-editor__row"
          data-testid="schema-row"
        >
          <div className="schema-editor__grid">
            <Input
              label="Key"
              value={field.key}
              placeholder="e.g. year"
              onChange={(e) => patch(index, { key: e.target.value })}
            />
            <Input
              label="Label"
              value={field.label}
              placeholder="e.g. Year"
              onChange={(e) => patch(index, { label: e.target.value })}
            />
            <Select
              label="Type"
              options={TYPE_OPTIONS}
              value={field.type}
              onChange={(e) =>
                patch(index, { type: e.target.value as MetadataFieldType })
              }
            />
          </div>

          {(field.type === "select" || field.type === "multiselect") && (
            <div className="schema-editor__grid">
              <Input
                label="Options (comma separated)"
                value={(field.options ?? []).join(", ")}
                placeholder="x264, x265, AV1"
                onChange={(e) =>
                  patch(index, { options: parseOptions(e.target.value) })
                }
              />
              {field.type === "multiselect" && (
                <Input
                  label="Max items"
                  type="number"
                  value={field.max_items != null ? String(field.max_items) : ""}
                  onChange={(e) =>
                    patch(index, { max_items: numOrUndef(e.target.value) })
                  }
                />
              )}
            </div>
          )}

          {field.type === "number" && (
            <div className="schema-editor__grid">
              <Input
                label="Min"
                type="number"
                value={field.min != null ? String(field.min) : ""}
                onChange={(e) =>
                  patch(index, { min: numOrUndef(e.target.value) })
                }
              />
              <Input
                label="Max"
                type="number"
                value={field.max != null ? String(field.max) : ""}
                onChange={(e) =>
                  patch(index, { max: numOrUndef(e.target.value) })
                }
              />
              <Checkbox
                label="Whole numbers only"
                checked={Boolean(field.integer)}
                onChange={(e) => patch(index, { integer: e.target.checked })}
              />
            </div>
          )}

          {field.type === "text" && (
            <div className="schema-editor__grid">
              <Input
                label="Max length"
                type="number"
                value={field.max_length != null ? String(field.max_length) : ""}
                onChange={(e) =>
                  patch(index, { max_length: numOrUndef(e.target.value) })
                }
              />
              <Input
                label="Pattern (regex)"
                value={field.pattern ?? ""}
                placeholder="optional"
                onChange={(e) =>
                  patch(index, { pattern: e.target.value || undefined })
                }
              />
            </div>
          )}

          <div className="schema-editor__row-footer">
            <Checkbox
              label="Required"
              checked={Boolean(field.required)}
              onChange={(e) => patch(index, { required: e.target.checked })}
            />
            <button
              type="button"
              className="admin-btn admin-btn--danger admin-btn--sm"
              onClick={() => removeField(index)}
            >
              Remove
            </button>
          </div>
        </div>
      ))}
    </div>
  );
}
