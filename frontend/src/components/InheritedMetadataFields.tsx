import {
  METADATA_TYPE_LABELS,
  metadataFieldDetails,
  type MetadataField,
} from "@/utils/metadata";
import "./metadata-fields-table.css";

interface InheritedMetadataFieldsProps {
  fields: MetadataField[];
}

/**
 * Read-only view of the metadata fields a category inherits from its parent
 * chain. The parent's *effective* schema already merges every ancestor level,
 * so this covers inheritance to any depth. Shown so an admin editing a
 * sub-category can see which fields already apply (and won't redefine them);
 * these are not editable here — they're changed on the category that owns them.
 */
export function InheritedMetadataFields({
  fields,
}: InheritedMetadataFieldsProps) {
  if (fields.length === 0) return null;

  return (
    <div className="fields-table" data-testid="inherited-fields">
      <div className="fields-table__header">
        <span className="form-label">Inherited Fields</span>
        <span className="fields-table__hint">
          From parent categories · read-only
        </span>
      </div>
      <div className="admin-table-scroll">
        <table className="admin-table">
          <thead>
            <tr>
              <th>Label</th>
              <th>Key</th>
              <th>Type</th>
              <th>Required</th>
              <th>Details</th>
            </tr>
          </thead>
          <tbody>
            {fields.map((f) => (
              <tr key={f.key} data-testid="inherited-field-row">
                <td className="admin-table__name">{f.label}</td>
                <td className="admin-muted">{f.key}</td>
                <td>{METADATA_TYPE_LABELS[f.type]}</td>
                <td>{f.required ? "Yes" : "—"}</td>
                <td className="admin-muted">{metadataFieldDetails(f)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
