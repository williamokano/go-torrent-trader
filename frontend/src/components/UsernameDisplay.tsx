import { Link } from "react-router-dom";
import { WarningBadge } from "@/components/WarningBadge";

interface UsernameDisplayProps {
  userId: number;
  username: string;
  warned?: boolean;
  noLink?: boolean;
  className?: string;
}

export function UsernameDisplay({
  userId,
  username,
  warned,
  noLink,
  className,
}: UsernameDisplayProps) {
  return (
    <span className={className ? `username-display ${className}` : "username-display"}>
      {noLink ? (
        <span>{username}</span>
      ) : (
        <Link to={`/user/${userId}`}>{username}</Link>
      )}
      <WarningBadge warned={warned} />
    </span>
  );
}
