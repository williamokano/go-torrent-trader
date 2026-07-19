import { Input, Select } from "@/components/form";
import type { MetadataField } from "@/utils/metadata";
import "./metadata-filter-controls.css";

interface MetadataFilterControlsProps {
  schema: MetadataField[];
  /** Current value of a filter query param (e.g. "meta_year__gte"). */
  get: (paramKey: string) => string;
  /** Commit a filter query param value ("" clears it). */
  set: (paramKey: string, value: string) => void;
}

/**
 * Renders browse/search filter controls for a category's metadata schema.
 * Query-param names mirror the backend contract (BE-3.13a):
 *   meta_<key>            — equality (select / multiselect / boolean)
 *   meta_<key>__gte/__lte — numeric range (number fields)
 *
 * Free-text fields are intentionally omitted: the backend matches by exact
 * JSONB containment, so an exact-match text box is rarely useful and would
 * refetch on every keystroke. Text fields remain filterable via the API.
 */
export function MetadataFilterControls({
  schema,
  get,
  set,
}: MetadataFilterControlsProps) {
  const filterable = schema.filter((f) => f.type !== "text");
  if (filterable.length === 0) return null;

  return (
    <div className="meta-filters" data-testid="meta-filters">
      {filterable.map((field) => {
        const key = field.key;

        if (field.type === "number") {
          return (
            <div key={key} className="meta-filters__range">
              <span className="meta-filters__range-label">{field.label}</span>
              <div className="meta-filters__range-inputs">
                <Input
                  label="Min"
                  type="number"
                  value={get(`meta_${key}__gte`)}
                  onChange={(e) => set(`meta_${key}__gte`, e.target.value)}
                />
                <Input
                  label="Max"
                  type="number"
                  value={get(`meta_${key}__lte`)}
                  onChange={(e) => set(`meta_${key}__lte`, e.target.value)}
                />
              </div>
            </div>
          );
        }

        if (field.type === "boolean") {
          return (
            <Select
              key={key}
              label={field.label}
              options={[
                { value: "", label: "Any" },
                { value: "true", label: "Yes" },
                { value: "false", label: "No" },
              ]}
              value={get(`meta_${key}`)}
              onChange={(e) => set(`meta_${key}`, e.target.value)}
            />
          );
        }

        // select or multiselect → single-value equality/containment
        const options = [
          { value: "", label: "Any" },
          ...(field.options ?? []).map((o) => ({ value: o, label: o })),
        ];
        return (
          <Select
            key={key}
            label={field.label}
            options={options}
            value={get(`meta_${key}`)}
            onChange={(e) => set(`meta_${key}`, e.target.value)}
          />
        );
      })}
    </div>
  );
}
