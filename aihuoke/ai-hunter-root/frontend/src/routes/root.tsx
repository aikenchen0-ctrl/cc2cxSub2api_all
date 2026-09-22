import { Link, Outlet } from "@tanstack/react-router";
import { LayoutDashboard, Plus, Settings, Users } from "lucide-react";
import { LanguageSwitcher } from "@/i18n/LanguageSwitcher";
import { useT } from "@/i18n/I18nProvider";

export function RootLayout() {
  const t = useT();

  return (
    <div className="min-h-screen bg-background">
      <header className="sticky top-0 z-50 w-full border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60">
        <div className="container mx-auto flex h-14 items-center px-4">
          <Link to="/" className="me-8 flex items-center gap-2 text-lg font-bold">
            <img
              src="/project-icon.jpg"
              alt="AI获客"
              className="h-5 w-5 rounded object-cover"
            />
            <span>{t("brand.name")}</span>
          </Link>
          <nav className="flex items-center gap-4 text-sm">
            <Link
              to="/"
              className="flex items-center gap-1.5 text-muted-foreground transition-colors hover:text-foreground [&.active]:text-foreground"
            >
              <LayoutDashboard className="h-4 w-4" />
              {t("nav.dashboard")}
            </Link>
            <Link
              to="/hunts/new"
              className="flex items-center gap-1.5 text-muted-foreground transition-colors hover:text-foreground [&.active]:text-foreground"
            >
              <Plus className="h-4 w-4" />
              {t("nav.newHunt")}
            </Link>
            <Link
              to="/licensed-finder"
              className="flex items-center gap-1.5 text-muted-foreground transition-colors hover:text-foreground [&.active]:text-foreground"
            >
              <Users className="h-4 w-4" />
              {t("nav.licensedFinder")}
            </Link>
          </nav>
          <div className="ms-auto flex items-center gap-4">
            <LanguageSwitcher />
            <Link
              to="/settings"
              className="flex items-center gap-1.5 text-sm text-muted-foreground transition-colors hover:text-foreground [&.active]:text-foreground"
            >
              <Settings className="h-4 w-4" />
              {t("nav.settings")}
            </Link>
          </div>
        </div>
      </header>
      <main className="container mx-auto px-4 py-8">
        <Outlet />
      </main>
    </div>
  );
}
