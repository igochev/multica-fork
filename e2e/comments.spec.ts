import { test, expect } from "@playwright/test";
import { createTestApi, loginAsDefault } from "./helpers";
import type { TestApiClient } from "./fixtures";

test.describe("Comments", () => {
  let api: TestApiClient;
  let issueId: string;

  test.beforeEach(async ({ page }) => {
    api = await createTestApi();
    const issue = await api.createIssue("E2E Comment Test " + Date.now());
    issueId = issue.id;
    await loginAsDefault(page);
  });

  test.afterEach(async () => {
    await api.cleanup();
  });

  test("can add a comment on an issue", async ({ page }) => {
    const issueLink = page.locator(`a[href$="/issues/${issueId}"]`);
    await expect(issueLink).toBeVisible({ timeout: 5000 });
    await issueLink.click();
    await page.waitForURL(/\/issues\/[\w-]+/);

    await expect(page.getByText("Properties")).toBeVisible();

    const commentText = "E2E comment " + Date.now();
    const commentInput = page.locator(".rich-text-editor .ProseMirror").last();
    await commentInput.click();
    await commentInput.fill(commentText);

    const submitBtn = page.locator(".mt-4 button").last();
    await expect(submitBtn).toBeEnabled();
    await submitBtn.click();

    await expect(page.getByText(commentText)).toBeVisible({ timeout: 5000 });
  });

  test("comment submit button is disabled when empty", async ({ page }) => {
    const issueLink = page.locator(`a[href$="/issues/${issueId}"]`);
    await expect(issueLink).toBeVisible({ timeout: 5000 });
    await issueLink.click();
    await page.waitForURL(/\/issues\/[\w-]+/);

    await expect(page.getByText("Properties")).toBeVisible();

    const submitBtn = page.locator(".mt-4 button").last();
    await expect(submitBtn).toBeDisabled();
  });
});
