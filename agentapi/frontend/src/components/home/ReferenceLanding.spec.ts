import { mount, type VueWrapper } from "@vue/test-utils";
import { createPinia, setActivePinia } from "pinia";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import ReferenceLanding from "./ReferenceLanding.vue";
import { downloadOptions } from "@/utils/ccswitch-downloads";
import { initializeTheme, isDark } from "@/agent/theme";

describe("reference homepage interactions", () => {
  let page: VueWrapper;
  beforeEach(() => {
    setActivePinia(createPinia());
    localStorage.clear();
    localStorage.setItem("theme", "light");
    initializeTheme();
    page = mount(ReferenceLanding, {
      attachTo: document.body,
      global: {
        stubs: {
          RouterLink: { props: ["to"], template: '<a :href="to"><slot /></a>' },
        },
      },
    });
  });
  afterEach(() => {
    page.unmount();
    localStorage.clear();
    document.documentElement.classList.remove("dark");
    isDark.value = false;
  });

  it("expands the full snapshot and filters desktop and mobile prices together", async () => {
    expect(page.findAll("tbody tr")).toHaveLength(8);
    await page.get(".pt-expand button").trigger("click");
    expect(page.findAll("tbody tr")).toHaveLength(55);
    expect(page.findAll(".price-cards .pc")).toHaveLength(55);
    const filter = page
      .findAll(".filters button")
      .find((button) => button.text() === "Anthropic")!;
    await filter.trigger("click");
    expect(filter.attributes("aria-pressed")).toBe("true");
    expect(page.findAll("tbody tr").length).toBeGreaterThan(0);
    expect(
      page
        .findAll("tbody tr")
        .every((row) => row.findAll("td")[1].text() === "Anthropic"),
    ).toBe(true);
    expect(page.findAll(".price-cards .pc")).toHaveLength(
      page.findAll("tbody tr").length,
    );
    expect(page.get(".pt-desc").text()).toContain("不代表本站售价");
  });

  it("opens one FAQ at a time and supports collapsing it", async () => {
    const questions = page.findAll(".faq-q");
    expect(questions[0].attributes("aria-expanded")).toBe("true");
    await questions[1].trigger("click");
    expect(questions[0].attributes("aria-expanded")).toBe("false");
    expect(questions[1].attributes("aria-expanded")).toBe("true");
    await questions[1].trigger("click");
    expect(page.findAll(".faq-item.open")).toHaveLength(0);
  });

  it("opens mobile navigation and closes it after choosing an anchor", async () => {
    expect(page.get(".mobile-menu").isVisible()).toBe(false);
    await page.get(".hamburger").trigger("click");
    expect(page.get(".mobile-menu").isVisible()).toBe(true);
    await page.get(".mobile-menu a").trigger("click");
    expect(page.get(".mobile-menu").isVisible()).toBe(false);
  });

  it("shares the global light and dark theme with the console", async () => {
    expect(page.attributes("data-theme")).toBe("light");
    await page.get(".tt-btn").trigger("click");
    expect(page.attributes("data-theme")).toBe("dark");
    expect(localStorage.getItem("theme")).toBe("dark");
    expect(document.documentElement.classList.contains("dark")).toBe(true);
  });

  it("uses this project’s downloads and never sends users to the source site", () => {
    const links = page
      .findAll("a")
      .map((link) => link.attributes("href") || "");
    for (const item of downloadOptions) expect(links).toContain(item.url);
    expect(links.some((href) => href.includes("aicodings.club"))).toBe(false);
    expect(page.text()).not.toMatch(/1\.5 折|99\.9%|免押金|10,000|自动还原/);
    expect(page.find("script").exists()).toBe(false);
    expect(page.find("iframe").exists()).toBe(false);
  });
});
