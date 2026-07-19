import { Input, Select, Checkbox } from "@/components/form";
import type { MetadataField, MetadataValues } from "@/utils/metadata";
import "./metadata-fields.css";

interface MetadataFieldsProps {
  schema: MetadataField[];
  values: MetadataValues;
  onChange: (values: MetadataValues) => void;
  disabled?: boolean;
}

/**
 * Renders category-defined metadata fields as the appropriate input per type.
 * Reused by the upload and edit torrent forms. Controlled: the parent owns the
 * values object and receives an updated copy on every change.
 */
export function MetadataFields({
  schema,
  values,
  onChange,
  disabled,
}: MetadataFieldsProps) {
  if (schema.length === 0) return null;

  const set = (key: string, value: unknown) => {
    onChange({ ...values, [key]: value });
  };

  return (
    <div className="metadata-fields">
      <h2 className="metadata-fields__title">Details</h2>
      {schema.map((field) => {
        const label = field.required ? `${field.label} *` : field.label;
        const value = values[field.key];

        if (field.type === "boolean") {
          return (
            <Checkbox
              key={field.key}
              label={label}
              checked={Boolean(value)}
              disabled={disabled}
              onChange={(e) => set(field.key, e.target.checked)}
            />
          );
        }

        if (field.type === "select") {
          const options = [
            { value: "", label: field.required ? "Select…" : "— None —" },
            ...(field.options ?? []).map((o) => ({ value: o, label: o })),
          ];
          return (
            <Select
              key={field.key}
              label={label}
              options={options}
              value={value != null ? String(value) : ""}
              disabled={disabled}
              onChange={(e) => set(field.key, e.target.value)}
            />
          );
        }

        if (field.type === "multiselect") {
          const selected = Array.isArray(value) ? (value as string[]) : [];
          return (
            <fieldset key={field.key} className="metadata-fields__group">
              <legend className="metadata-fields__legend">{label}</legend>
              {field.help && (
                <p className="metadata-fields__help">{field.help}</p>
              )}
              <div className="metadata-fields__options">
                {(field.options ?? []).map((opt) => {
                  const checked = selected.includes(opt);
                  return (
                    <Checkbox
                      key={opt}
                      label={opt}
                      checked={checked}
                      disabled={disabled}
                      onChange={(e) => {
                        const next = e.target.checked
                          ? [...selected, opt]
                          : selected.filter((s) => s !== opt);
                        set(field.key, next);
                      }}
                    />
                  );
                })}
              </div>
            </fieldset>
          );
        }

        // text or number
        return (
          <div key={field.key}>
            <Input
              label={label}
              type={field.type === "number" ? "number" : "text"}
              value={value != null ? String(value) : ""}
              disabled={disabled}
              min={field.min}
              max={field.max}
              maxLength={field.type === "text" ? field.max_length : undefined}
              onChange={(e) => set(field.key, e.target.value)}
            />
            {field.help && (
              <p className="metadata-fields__help">{field.help}</p>
            )}
          </div>
        );
      })}
    </div>
  );
}
