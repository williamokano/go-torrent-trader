export function AdminRoute({ children }: { children: React.ReactNode }) {
  // TODO: check if user is admin, redirect to / if not
  return <>{children}</>;
}
