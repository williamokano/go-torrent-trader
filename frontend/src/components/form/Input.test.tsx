import { render, screen, cleanup } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { Input } from "@/components/form/Input";

describe("Input", () => {
  afterEach(() => {
    cleanup();
  });

  it("renders label and input", () => {
    render(<Input label="Username" />);
    expect(screen.getByLabelText("Username")).toBeInTheDocument();
    expect(screen.getByLabelText("Username").tagName).toBe("INPUT");
  });

  it("shows error message when error prop is set", () => {
    render(<Input label="Email" error="Email is required" />);
    expect(screen.getByRole("alert")).toHaveTextContent("Email is required");
    expect(screen.getByLabelText("Email")).toHaveAttribute(
      "aria-invalid",
      "true",
    );
  });

  it("does not show error when error prop is not set", () => {
    render(<Input label="Name" />);
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("forwards HTML attributes", () => {
    render(
      <Input
        label="Password"
        placeholder="Enter password"
        disabled
        type="password"
      />,
    );
    const input = screen.getByLabelText("Password");
    expect(input).toHaveAttribute("placeholder", "Enter password");
    expect(input).toBeDisabled();
    expect(input).toHaveAttribute("type", "password");
  });
});
