import { render, screen } from "@testing-library/react";
import { beforeEach, test, expect } from "vitest";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { AuthProvider } from "@/features/auth";
import { AdminRoute } from "@/routes/AdminRoute";
import { clearTokens } from "@/features/auth/token";

beforeEach(() => {
  clearTokens();
  localStorage.clear();
});

test("AdminRoute redirects to login when not authenticated", () => {
  render(
    <AuthProvider>
      <MemoryRouter initialEntries={["/admin"]}>
        <Routes>
          <Route
            path="/admin"
            element={
              <AdminRoute>
                <p>Admin content</p>
              </AdminRoute>
            }
          />
          <Route path="/login" element={<p>Login page</p>} />
        </Routes>
      </MemoryRouter>
    </AuthProvider>,
  );

  expect(screen.queryByText("Admin content")).not.toBeInTheDocument();
  screen.getByText("Login page");
});
