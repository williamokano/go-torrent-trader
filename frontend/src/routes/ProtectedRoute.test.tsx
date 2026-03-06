import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, test, expect } from "vitest";
import { ProtectedRoute } from "@/routes/ProtectedRoute";

afterEach(cleanup);

describe("ProtectedRoute", () => {
  test("renders children", () => {
    render(
      <ProtectedRoute>
        <div>Protected content</div>
      </ProtectedRoute>,
    );
    expect(screen.getByText("Protected content")).toBeInTheDocument();
  });
});
