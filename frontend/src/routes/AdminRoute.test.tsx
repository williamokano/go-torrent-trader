import { render, screen } from "@testing-library/react";
import { test } from "vitest";
import { AdminRoute } from "@/routes/AdminRoute";

test("renders children", () => {
  render(
    <AdminRoute>
      <p>Admin content</p>
    </AdminRoute>,
  );
  screen.getByText("Admin content");
});
