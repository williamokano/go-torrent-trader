import { useId } from "react";
import "@/components/form/form.css";

interface SelectProps extends React.SelectHTMLAttributes<HTMLSelectElement> {
  label: string;
  error?: string;
  options: { value: string; label: string }[];
}

export function Select({
  label,
  error,
  options,
  id,
  className,
  ...rest
}: SelectProps) {
  const generatedId = useId();
  const selectId = id ?? generatedId;
  const errorId = error ? `${selectId}-error` : undefined;

  return (
    <div className="form-field">
      <label className="form-label" htmlFor={selectId}>
        {label}
      </label>
      <select
        id={selectId}
        className={`form-select${error ? " form-select--error" : ""}${className ? ` ${className}` : ""}`}
        aria-invalid={error ? true : undefined}
        aria-describedby={errorId}
        {...rest}
      >
        {options.map((option) => (
          <option key={option.value} value={option.value}>
            {option.label}
          </option>
        ))}
      </select>
      {error && (
        <span id={errorId} className="form-error" role="alert">
          {error}
        </span>
      )}
    </div>
  );
}
