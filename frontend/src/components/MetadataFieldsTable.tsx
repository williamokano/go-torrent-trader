import { useState } from "react";
import { MetadataFieldModal } from "@/components/MetadataFieldModal";
import type { MetadataField, MetadataFieldType } from "@/utils/metadata";
import "./metadata-fields-table.css";

interface MetadataFieldsTableProps {
  value: MetadataField[];
  onChange: (fields: MetadataField[]) => void;
}

const TYPE_LABELS: Record<MetadataFieldType, string> = {
  text: "Text",
  number: "Number",
  select: "Select",
  multiselect: "Multi-select",
  boolean: "Checkbox",
};

// fieldDetails summarizes a field's type-specific constraints for the table.
function fieldDetails(f: MetadataField): string {
  switch (f.type) {
    case "select":
    case "multiselect": {
      const opts = (f.options ?? []).join(", ");
      const max =
        f.type === "multiselect" && f.max_items != null
          ? ` (max ${f.max_items})`
          : "";
      return opts ? `${opts}${max}` : "—";
    }
    case "number": {
      const parts: string[] = [];
      if (f.min != null) parts.push(`min ${f.min}`);
      if (f.max != null) parts.push(`max ${f.max}`);
      if (f.integer) parts.push("whole");
      return parts.length > 0 ? parts.join(", ") : "—";
    }
    case "text": {
      const parts: string[] = [];
      if (f.max_length != null) parts.push(`max length ${f.max_length}`);
      if (f.pattern) parts.push(`pattern ${f.pattern}`);
      return parts.length > 0 ? parts.join(", ") : "—";
    }
    default:
      return "—";
  }
}

/**
 * Table view of a category's metadata field definitions. Adding or editing a
 * field opens {@link MetadataFieldModal}; on save the field appears as a row.
 * Emits the whole `metadata_schema` array via onChange; the backend validates
 * it when the category is saved.
 */
export function MetadataFieldsTable({
  value,
  onChange,
}: MetadataFieldsTableProps) {
  // index === null while adding; a number while editing that row.
  const [editing, setEditing] = useState<{ index: number | null } | null>(null);

  const closeModal = () => setEditing(null);

  const saveField = (field: MetadataField) => {
    if (editing?.index == null) {
      onChange([...value, field]);
    } else {
      const idx = editing.index;
      onChange(value.map((f, i) => (i === idx ? field : f)));
    }
    closeModal();
  };

  const removeField = (index: number) => {
    onChange(value.filter((_, i) => i !== index));
  };

  const existingKeys =
    editing?.index == null
      ? value.map((f) => f.key)
      : value.filter((_, i) => i !== editing.index).map((f) => f.key);

  return (
    <div className="fields-table">
      <div className="fields-table__header">
        <span className="form-label">Metadata Fields</span>
        <button
          type="button"
          className="admin-btn admin-btn--ghost admin-btn--sm"
          onClick={() => setEditing({ index: null })}
        >
          Add Field
        </button>
      </div>

      {value.length === 0 ? (
        <p className="fields-table__empty">
          No custom fields. Uploads in this category collect only the standard
          fields.
        </p>
      ) : (
        <div className="admin-table-scroll">
          <table className="admin-table">
            <thead>
              <tr>
                <th>Label</th>
                <th>Key</th>
                <th>Type</th>
                <th>Required</th>
                <th>Details</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {value.map((f, i) => (
                <tr key={i} data-testid="field-row">
                  <td className="admin-table__name">{f.label}</td>
                  <td className="admin-muted">{f.key}</td>
                  <td>{TYPE_LABELS[f.type]}</td>
                  <td>{f.required ? "Yes" : "—"}</td>
                  <td className="admin-muted">{fieldDetails(f)}</td>
                  <td className="admin-table__actions">
                    <button
                      type="button"
                      className="admin-btn admin-btn--ghost admin-btn--sm"
                      onClick={() => setEditing({ index: i })}
                    >
                      Edit
                    </button>{" "}
                    <button
                      type="button"
                      className="admin-btn admin-btn--danger admin-btn--sm"
                      onClick={() => removeField(i)}
                    >
                      Remove
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {editing && (
        <MetadataFieldModal
          initial={editing.index != null ? value[editing.index] : null}
          existingKeys={existingKeys}
          onSave={saveField}
          onClose={closeModal}
        />
      )}
    </div>
  );
}
