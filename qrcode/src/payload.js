const FIXED_MODEL = "gpt-4.1-mini";

export function buildResponsesPayload({ chatText, imageDataUrls }) {
  const content = [
    {
      type: "input_text",
      text: [
        "请把下面的聊天记录整理为可执行脚本。",
        "输出必须使用简体中文。",
        "请提炼目标、素材、视觉要点、限制条件、执行步骤和可直接复用的最终提示词。",
        "",
        "聊天记录：",
        chatText
      ].join("\n")
    }
  ];

  for (const imageDataUrl of imageDataUrls) {
    content.push({
      type: "input_image",
      image_url: imageDataUrl
    });
  }

  return {
    model: FIXED_MODEL,
    input: [
      {
        role: "user",
        content
      }
    ]
  };
}

export { FIXED_MODEL };
