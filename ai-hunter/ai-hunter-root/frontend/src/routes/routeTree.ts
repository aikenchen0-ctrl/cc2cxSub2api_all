import { createRootRoute, createRoute } from "@tanstack/react-router";
import { RootLayout } from "./root";
import { DashboardPage } from "./dashboard";
import { NewHuntPage } from "./new-hunt";
import { HuntDetailPage } from "./hunt-detail";
import { AutomationJobPage } from "./automation-job";
import { SettingsPage } from "./settings";
import { LicensedFinderPage } from "./licensed-finder";

const rootRoute = createRootRoute({ component: RootLayout });

const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/",
  component: DashboardPage,
});

const newHuntRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/hunts/new",
  component: NewHuntPage,
});

const huntDetailRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/hunts/$huntId",
  component: HuntDetailPage,
});

const automationJobRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/automation/$jobId",
  component: AutomationJobPage,
});

const settingsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/settings",
  component: SettingsPage,
});

const licensedFinderRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/licensed-finder",
  component: LicensedFinderPage,
});

export const routeTree = rootRoute.addChildren([
  indexRoute,
  newHuntRoute,
  huntDetailRoute,
  automationJobRoute,
  settingsRoute,
  licensedFinderRoute,
]);
