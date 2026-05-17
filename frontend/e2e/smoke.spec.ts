import { test, expect } from "@playwright/test";

test("cockpit shell renders", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByText("Agent Fleet Cockpit")).toBeVisible();
});
