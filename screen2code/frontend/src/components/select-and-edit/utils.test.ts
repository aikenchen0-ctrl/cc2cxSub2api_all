import {
  buildSelectedElementInstruction,
  describeElementContext,
} from "./utils";

// Minimal stand-in for a DOM element; jest runs in the node environment.
interface FakeElement {
  tagName: string;
  outerHTML: string;
  parentElement: FakeElement | null;
  ownerDocument: { getElementsByTagName: (tag: string) => FakeElement[] };
  getAttribute: (name: string) => string | null;
}

function fakeElement(tag: string, classAttr: string, outerHTML = ""): FakeElement {
  return {
    tagName: tag.toUpperCase(),
    outerHTML,
    parentElement: null,
    ownerDocument: { getElementsByTagName: () => [] },
    getAttribute: (name) => (name === "class" && classAttr ? classAttr : null),
  };
}

function asElement(el: FakeElement): Element {
  return el as unknown as Element;
}

describe("describeElementContext", () => {
  function buildPricingPage() {
    const body = fakeElement("body", "");
    const grid = fakeElement("div", "pricing-grid");
    const card = fakeElement("div", "pricing-card featured");
    const anchorHtml = '<a href="#" class="btn">Choose plan</a>';
    const basicBtn = fakeElement("a", "btn", anchorHtml);
    const proBtn = fakeElement("a", "btn", anchorHtml);
    const enterpriseBtn = fakeElement("a", "btn", anchorHtml);
    grid.parentElement = body;
    card.parentElement = grid;
    proBtn.parentElement = card;
    const doc = {
      getElementsByTagName: () => [basicBtn, proBtn, enterpriseBtn],
    };
    [basicBtn, proBtn, enterpriseBtn].forEach((el) => {
      el.ownerDocument = doc;
    });
    return { proBtn };
  }

  it("builds an ancestor path with classes", () => {
    const { proBtn } = buildPricingPage();
    const context = describeElementContext(asElement(proBtn));
    expect(context).toContain(
      "元素位置：body > div.pricing-grid > div.pricing-card.featured > a.btn"
    );
  });

  it("notes the position among identical elements", () => {
    const { proBtn } = buildPricingPage();
    const context = describeElementContext(asElement(proBtn));
    expect(context).toContain("页面上有 3 个元素使用完全相同的标记");
    expect(context).toContain("第 2/3 个");
  });

  it("omits the duplicate note when the markup is unique", () => {
    const heading = fakeElement("h1", "title", "<h1 class=\"title\">Hi</h1>");
    heading.ownerDocument = { getElementsByTagName: () => [heading] };
    const context = describeElementContext(asElement(heading));
    expect(context).toContain("元素位置：h1.title");
    expect(context).not.toContain("使用完全相同的标记");
  });
});

describe("buildSelectedElementInstruction", () => {
  it("includes the instruction and the element html", () => {
    const result = buildSelectedElementInstruction(
      "Make the button red",
      '<button class="btn">Buy</button>'
    );
    expect(result).toContain("Make the button red");
    expect(result).toContain('<button class="btn">Buy</button>');
    expect(result).toContain("在预览中选中");
  });

  it("mentions that the snippet is rendered DOM, not source", () => {
    const result = buildSelectedElementInstruction(
      "Center it",
      "<div>x</div>"
    );
    expect(result).toContain("从渲染页面获取的元素 outerHTML");
  });

  it("truncates very large element html", () => {
    const hugeHtml = `<div>${"a".repeat(20000)}</div>`;
    const result = buildSelectedElementInstruction("Shrink it", hugeHtml);
    expect(result).toContain("内容已截断");
    expect(result.length).toBeLessThan(hugeHtml.length);
  });

  it("does not truncate small element html", () => {
    const result = buildSelectedElementInstruction(
      "Bold it",
      "<span>hello</span>"
    );
    expect(result).not.toContain("内容已截断");
  });

  it("includes the element context when provided", () => {
    const result = buildSelectedElementInstruction(
      "Make it red",
      '<a class="btn">Go</a>',
      "元素位置：body > div.card > a.btn"
    );
    expect(result).toContain("元素位置：body > div.card > a.btn");
  });

  it("omits the context block when not provided", () => {
    const result = buildSelectedElementInstruction(
      "Make it red",
      '<a class="btn">Go</a>'
    );
    expect(result).not.toContain("Element location:");
  });
});
