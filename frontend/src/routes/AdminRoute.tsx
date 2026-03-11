import { useAuth } from "@/features/auth";
import { Navigate, useLocation } from "react-router-dom";

export function AdminRoute({ children }: { children: React.ReactNode }) {
  const { isAuthenticated, isLoading, user } = useAuth();
  const location = useLocation();

  if (isLoading) return <div>Loading...</div>;
  if (!isAuthenticated)
    return <Navigate to="/login" state={{ from: location }} replace />;
  if (!user?.isStaff) return <Navigate to="/" replace />;
  return <>{children}</>;
}

export function AdminIndexRedirect() {
  const { user } = useAuth();
  if (!user?.isAdmin) return <Navigate to="/admin/chat-mutes" replace />;
  return null;
}
