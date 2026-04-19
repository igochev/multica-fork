import { createHmac, randomBytes } from "node:crypto";
import { test, type Page } from "@playwright/test";
import { TestApiClient } from "./fixtures";

type E2EIdentity = {
  email: string;
  name: string;
  workspaceName: string;
  workspaceSlug: string;
};

function slugify(value: string) {
  return value
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 32);
}

function getE2EIdentity(): E2EIdentity {
  const info = test.info();
  const fileStem = info.file.split("/").pop()?.replace(/\.spec\.ts$/, "") ?? "e2e";
  const titleStem = slugify(info.title || "default") || "default";
  const suffix = `${slugify(fileStem)}-${titleStem}-${info.workerIndex}`.slice(0, 48);

  return {
    email: `e2e.${suffix}@multica.ai`,
    name: `E2E User ${suffix}`,
    workspaceName: `E2E Workspace ${suffix}`,
    workspaceSlug: `e2e-${suffix}`,
  };
}

function generateCsrfToken(authToken: string) {
  const nonce = randomBytes(16);
  const nonceHex = nonce.toString("hex");
  const signature = createHmac("sha256", authToken).update(nonce).digest("hex");
  return `${nonceHex}.${signature}`;
}

function getBaseUrl() {
  return process.env.PLAYWRIGHT_BASE_URL ?? process.env.FRONTEND_ORIGIN ?? "http://localhost:3000";
}

function getApiBaseUrl() {
  return process.env.NEXT_PUBLIC_API_URL || `http://localhost:${process.env.PORT || "8080"}`;
}

function escapeRegex(value: string) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

/**
 * Log in as the current test's isolated E2E user and ensure its workspace exists.
 * Authenticates via API (send-code → DB read → verify-code), then seeds the
 * browser auth state the same way the current Next.js proxy expects.
 *
 * Returns the workspace slug so callers can build workspace-scoped URLs.
 */
export async function loginAsDefault(page: Page): Promise<string> {
  const identity = getE2EIdentity();
  const api = new TestApiClient();
  await api.login(identity.email, identity.name);
  const workspace = await api.ensureWorkspace(
    identity.workspaceName,
    identity.workspaceSlug,
  );

  const token = api.getToken();
  const csrfToken = generateCsrfToken(token);
  const baseURL = getBaseUrl();
  const apiBaseURL = getApiBaseUrl();

  await page.context().addCookies([
    {
      name: "multica_auth",
      value: token,
      url: apiBaseURL,
      httpOnly: true,
      sameSite: "Strict",
    },
    {
      name: "multica_csrf",
      value: csrfToken,
      url: apiBaseURL,
      sameSite: "Strict",
    },
    {
      name: "multica_logged_in",
      value: "1",
      url: baseURL,
      sameSite: "Lax",
    },
    {
      name: "last_workspace_slug",
      value: workspace.slug,
      url: baseURL,
      sameSite: "Lax",
    },
  ]);
  await page.addInitScript((loginToken) => {
    localStorage.setItem("multica_token", loginToken);
  }, token);
  await page.goto(`/${workspace.slug}/issues`);
  await page.waitForURL(
    new RegExp(`/${escapeRegex(workspace.slug)}/issues(?:$|\\?)`),
    {
      timeout: 10000,
    },
  );
  return workspace.slug;
}

/**
 * Create a TestApiClient logged in as the current test's isolated E2E user.
 * Call api.cleanup() in afterEach to remove test data created during the test.
 */
export async function createTestApi(): Promise<TestApiClient> {
  const identity = getE2EIdentity();
  const api = new TestApiClient();
  await api.login(identity.email, identity.name);
  await api.ensureWorkspace(identity.workspaceName, identity.workspaceSlug);
  return api;
}

export async function openWorkspaceMenu(page: Page) {
  const workspaceButton = page.getByRole("button", {
    name: /E2E Workspace/i,
  }).first();
  await workspaceButton.click();
  await page.locator('[class*="popover"]').waitFor({ state: "visible" });
}
