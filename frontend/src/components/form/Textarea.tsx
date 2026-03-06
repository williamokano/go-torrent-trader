import { useId } from "react";
import "@/components/form/form.css";

interface TextareaProps extends React.TextareaHTMLAttributes<HTMLTextAreaElement> {
  label: string;
  error?: string;
}

export function Textarea({
  label,
  error,
  id,
  className,
  ...rest
}: TextareaProps) {
  const generatedId = useId();
  const textareaId = id ?? generatedId;
  const errorId = error ? `${textareaId}-error` : undefined;

  return (
    <div className="form-field">
      <label className="form-label" htmlFor={textareaId}>
        {label}
      </label>
      <textarea
        id={textareaId}
        className={`form-textarea${error ? " form-textarea--error" : ""}${className ? ` ${className}` : ""}`}
        aria-invalid={error ? true : undefined}
        aria-describedby={errorId}
        {...rest}
      />
      {error && (
        <span id={errorId} className="form-error" role="alert">
          {error}
        </span>
      )}
    </div>
  );
}
