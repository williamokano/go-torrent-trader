import { Link } from "react-router-dom";
import { WarningBadge } from "@/components/WarningBadge";

interface UsernameDisplayProps {
  // Kept on the props type (rather than removed) so the ~20 existing call
  // sites that already pass it don't need touching — the link now targets
  // the username directly and no longer needs the numeric id.
  userId: number;
  username: string;
  warned?: boolean;
  noLink?: boolean;
  className?: string;
}

export function UsernameDisplay({
  username,
  warned,
  noLink,
  className,
}: UsernameDisplayProps) {
  return (
    <span
      className={
        className ? `username-display ${className}` : "username-display"
      }
    >
      {noLink ? (
        <span>{username}</span>
      ) : (
        <Link to={`/user/${username}`}>{username}</Link>
      )}
      <WarningBadge warned={warned} />
    </span>
  );
}
