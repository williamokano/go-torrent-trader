import { render, screen } from "@testing-library/react";
import { beforeEach, test, expect } from "vitest";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { AuthProvider } from "@/features/auth";
import { ProtectedRoute } from "@/routes/ProtectedRoute";
import { clearTokens } from "@/features/auth/token";

beforeEach(() => {
  clearTokens();
  localStorage.clear();
});

test("ProtectedRoute redirects to login when not authenticated", () => {
  render(
    <AuthProvider>
      <MemoryRouter initialEntries={["/protected"]}>
        <Routes>
          <Route
            path="/protected"
            element={
              <ProtectedRoute>
                <p>Protected content</p>
              </ProtectedRoute>
            }
          />
          <Route path="/login" element={<p>Login page</p>} />
        </Routes>
      </MemoryRouter>
    </AuthProvider>,
  );

  expect(screen.queryByText("Protected content")).not.toBeInTheDocument();
  screen.getByText("Login page");
});
