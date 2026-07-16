import { useId, useState } from "react";
import "@/components/form/form.css";

// 1024-based to match formatBytes in utils/format.
const BYTE_UNITS = [
  { label: "B", factor: 1 },
  { label: "KB", factor: 1024 },
  { label: "MB", factor: 1024 ** 2 },
  { label: "GB", factor: 1024 ** 3 },
  { label: "TB", factor: 1024 ** 4 },
] as const;

interface ByteSizeInputProps {
  label: string;
  /** Canonical value in bytes. */
  value: number;
  onChange: (bytes: number) => void;
  id?: string;
  disabled?: boolean;
}

interface SizeState {
  bytes: number;
  amount: string;
  unit: number;
}

function bestUnit(bytes: number): number {
  if (bytes <= 0) return 0;
  let i = 0;
  while (i + 1 < BYTE_UNITS.length && bytes >= BYTE_UNITS[i + 1].factor) i++;
  return i;
}

function formatAmount(bytes: number, unit: number): string {
  const n = bytes / BYTE_UNITS[unit].factor;
  // Up to 4 decimals, trailing zeros trimmed. Display only — the canonical
  // bytes value never changes unless the admin types a new amount.
  return String(Number(n.toFixed(4)));
}

function derive(bytes: number): SizeState {
  const unit = bestUnit(bytes);
  return { bytes, amount: formatAmount(bytes, unit), unit };
}

/**
 * A size field that edits in human units instead of raw bytes. The number
 * input and unit select work on a display copy; the exact byte value is the
 * source of truth and only moves when the admin types a new amount, so
 * opening and saving a form never silently rounds a stored value.
 */
export function ByteSizeInput({
  label,
  value,
  onChange,
  id,
  disabled,
}: ByteSizeInputProps) {
  const generatedId = useId();
  const inputId = id ?? generatedId;
  const [state, setState] = useState<SizeState>(() => derive(value));

  // Re-derive when the parent supplies a different value (e.g. a refetch).
  if (value !== state.bytes) {
    setState(derive(value));
  }

  const handleAmountChange = (raw: string) => {
    const parsed = Number.parseFloat(raw);
    if (!Number.isFinite(parsed)) {
      // Mid-edit emptiness must not zero the stored value; the canonical
      // bytes stay put and blur restores the display from them.
      setState({ ...state, amount: raw });
      return;
    }
    const bytes = Math.min(
      Math.max(Math.round(parsed * BYTE_UNITS[state.unit].factor), 0),
      Number.MAX_SAFE_INTEGER,
    );
    setState({ ...state, amount: raw, bytes });
    onChange(bytes);
  };

  const handleBlur = () => {
    if (!Number.isFinite(Number.parseFloat(state.amount))) {
      setState({ ...state, amount: formatAmount(state.bytes, state.unit) });
    }
  };

  const handleUnitChange = (unit: number) => {
    setState({ ...state, unit, amount: formatAmount(state.bytes, unit) });
  };

  const hintId = `${inputId}-hint`;

  return (
    <div className="form-field">
      <label className="form-label" htmlFor={inputId}>
        {label}
      </label>
      <div className="form-sizegroup">
        <input
          id={inputId}
          className="form-input form-sizegroup__amount"
          type="number"
          min="0"
          step="any"
          inputMode="decimal"
          value={state.amount}
          onChange={(e) => handleAmountChange(e.target.value)}
          onBlur={handleBlur}
          aria-describedby={state.unit > 0 ? hintId : undefined}
          disabled={disabled}
        />
        <select
          className="form-select form-sizegroup__unit"
          aria-label={`${label} unit`}
          value={state.unit}
          onChange={(e) => handleUnitChange(Number(e.target.value))}
          disabled={disabled}
        >
          {BYTE_UNITS.map((u, i) => (
            <option key={u.label} value={i}>
              {u.label}
            </option>
          ))}
        </select>
      </div>
      {state.unit > 0 && (
        <span id={hintId} className="form-hint">
          = {state.bytes.toLocaleString()} bytes
        </span>
      )}
    </div>
  );
}
