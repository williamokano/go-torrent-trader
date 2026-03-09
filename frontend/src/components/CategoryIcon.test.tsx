import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, test } from "vitest";
import { CategoryIcon } from "@/components/CategoryIcon";

afterEach(cleanup);

describe("CategoryIcon", () => {
  test("renders placeholder with first letter when no imageUrl", () => {
    render(<CategoryIcon name="Movies" />);
    expect(screen.getByText("M")).toBeInTheDocument();
  });

  test("renders image when imageUrl is provided", () => {
    render(
      <CategoryIcon name="Games" imageUrl="https://example.com/icon.png" />,
    );
    const img = screen.getByRole("img", { name: "Games" });
    expect(img).toBeInTheDocument();
    expect(img).toHaveAttribute("src", "https://example.com/icon.png");
  });

  test("renders placeholder when imageUrl is null", () => {
    render(<CategoryIcon name="TV Shows" imageUrl={null} />);
    expect(screen.getByText("T")).toBeInTheDocument();
  });

  test("applies size class", () => {
    const { container } = render(<CategoryIcon name="Music" size="lg" />);
    const el = container.querySelector(".category-icon--lg");
    expect(el).toBeInTheDocument();
  });

  test("applies custom className", () => {
    const { container } = render(
      <CategoryIcon name="Software" className="my-custom" />,
    );
    const el = container.querySelector(".my-custom");
    expect(el).toBeInTheDocument();
  });
});
