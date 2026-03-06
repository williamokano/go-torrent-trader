import { useId } from "react";
import "@/components/form/form.css";

interface CheckboxProps extends React.InputHTMLAttributes<HTMLInputElement> {
  label: string;
}

export function Checkbox({ label, id, className, ...rest }: CheckboxProps) {
  const generatedId = useId();
  const checkboxId = id ?? generatedId;

  return (
    <div className="form-field form-field--checkbox">
      <input
        type="checkbox"
        id={checkboxId}
        className={`form-checkbox${className ? ` ${className}` : ""}`}
        {...rest}
      />
      <label className="form-label" htmlFor={checkboxId}>
        {label}
      </label>
    </div>
  );
}
