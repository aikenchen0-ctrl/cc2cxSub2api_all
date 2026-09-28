import * as z from "zod";

export const ImageSchema = z.object({
  __image_url__: z.url().meta({
    description: "图片地址",
  }),
  __image_prompt__: z
    .string()
    .meta({
      description: "用于生成图片的提示词",
    })
    .min(10)
    .max(50),
});

export const IconSchema = z.object({
  __icon_url__: z.string().meta({
    description: "图标地址",
  }),
  __icon_query__: z
    .string()
    .meta({
      description: "用于搜索图标的关键词",
    })
    .min(5)
    .max(20),
});
