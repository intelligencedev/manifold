import { test, expect } from "@playwright/test";

test("renders overview headline", async ({ page }) => {
  await page.goto("/overview");
  await expect(page.getByRole("heading", { name: "Agents" })).toBeVisible();
});

test("renders the dedicated realtime voice view", async ({ page }) => {
  await page.goto("/realtime");
  await expect(
    page.getByRole("heading", { name: "Realtime voice" }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Start conversation" }),
  ).toBeVisible();
});
