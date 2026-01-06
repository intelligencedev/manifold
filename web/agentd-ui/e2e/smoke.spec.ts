import { test, expect } from "@playwright/test";

test("renders overview headline", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByRole("heading", { name: "Agents" })).toBeVisible();
});
