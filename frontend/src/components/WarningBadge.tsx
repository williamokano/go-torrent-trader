import "./warning-badge.css";

interface WarningBadgeProps {
  warned?: boolean;
}

/**
 * Renders a small warning indicator next to a username.
 * Only renders when warned is true.
 */
export function WarningBadge({ warned }: WarningBadgeProps) {
  if (!warned) return null;

  return (
    <span className="warning-badge" title="This user has an active warning">
      !
    </span>
  );
}
