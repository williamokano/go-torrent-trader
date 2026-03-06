import { render, screen, cleanup } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { Select } from "@/components/form/Select";

const options = [
  { value: "a", label: "Option A" },
  { value: "b", label: "Option B" },
  { value: "c", label: "Option C" },
];

describe("Select", () => {
  afterEach(() => {
    cleanup();
  });

  it("renders label and options", () => {
    render(<Select label="Category" options={options} />);
    expect(screen.getByLabelText("Category")).toBeInTheDocument();
    expect(screen.getByLabelText("Category").tagName).toBe("SELECT");
    expect(screen.getByText("Option A")).toBeInTheDocument();
    expect(screen.getByText("Option B")).toBeInTheDocument();
    expect(screen.getByText("Option C")).toBeInTheDocument();
  });

  it("shows error message when error prop is set", () => {
    render(
      <Select label="Priority" options={options} error="Please select one" />,
    );
    expect(screen.getByRole("alert")).toHaveTextContent("Please select one");
    expect(screen.getByLabelText("Priority")).toHaveAttribute(
      "aria-invalid",
      "true",
    );
  });

  it("does not show error when error prop is not set", () => {
    render(<Select label="Status" options={options} />);
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });
});
