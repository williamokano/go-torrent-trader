export function ProtectedRoute({ children }: { children: React.ReactNode }) {
  // TODO: check auth context, redirect to /login if not authenticated
  return <>{children}</>;
}
