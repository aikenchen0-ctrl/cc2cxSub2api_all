// Cap the element HTML included in the prompt. The model already receives the
// full current code, so the snippet only needs to identify the element.
const MAX_ELEMENT_HTML_LENGTH = 12000;

const MAX_PATH_DEPTH = 6;

function describeNode(el: Element): string {
  const tag = el.tagName.toLowerCase();
  const classAttr = el.getAttribute("class") || "";
  const classes = classAttr.split(/\s+/).filter(Boolean).slice(0, 3);
  return tag + classes.map((c) => `.${c}`).join("");
}

// The outerHTML alone can't identify the element when siblings share identical
// markup (e.g. three "Choose plan" buttons styled by a parent class), so also
// describe where it sits in the DOM.
export function describeElementContext(el: Element): string {
  const parts: string[] = [];
  let current: Element | null = el;
  while (current && parts.length < MAX_PATH_DEPTH) {
    if (current.tagName.toLowerCase() === "html") break;
    parts.unshift(describeNode(current));
    current = current.parentElement;
  }
  const lines = [`元素位置：${parts.join(" > ")}`];

  const identical = Array.from(
    el.ownerDocument.getElementsByTagName(el.tagName)
  ).filter((other) => other.outerHTML === el.outerHTML);
  if (identical.length > 1) {
    const position = identical.indexOf(el) + 1;
    lines.push(
      `页面上有 ${identical.length} 个元素使用完全相同的标记；用户选择的是按文档顺序排列的第 ${position}/${identical.length} 个。只编辑这一项，其他副本保持不变。由于标记重复，不要仅凭元素自身标记定位；请结合唯一的周围上下文（父元素或上方位置路径中的区分性祖先类名）定位，或通过该祖先限定样式范围。搜索/替换文本若匹配多个位置会改错副本。`
    );
  }
  return lines.join("\n");
}

export function buildSelectedElementInstruction(
  instruction: string,
  elementHtml: string,
  elementContext?: string
): string {
  const truncated =
    elementHtml.length > MAX_ELEMENT_HTML_LENGTH
      ? elementHtml.slice(0, MAX_ELEMENT_HTML_LENGTH) +
        "<!-- 内容已截断；请在当前代码中定位元素 -->"
      : elementHtml;

  return `${instruction}

请将修改应用到用户在预览中选中的这个具体元素：

\`\`\`html
${truncated}
\`\`\`
${elementContext ? `\n${elementContext}\n` : ""}
这段内容是从渲染页面获取的元素 outerHTML，可能与源代码不同（例如 JSX 使用 className，Vue 模板使用指令和插值，Ionic 或 Bootstrap 等框架可能在运行时注入类名或属性）。请找到生成该元素的代码并在那里修改，保持无关代码不变。`;
}
