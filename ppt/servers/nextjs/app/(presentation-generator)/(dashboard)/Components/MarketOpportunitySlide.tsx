import * as z from "zod";

export const slideLayoutId = "product-overview-market-opportunity-slide";
export const slideLayoutName = "产品概览与市场机会";
export const slideLayoutDescription =
  "市场机会幻灯片：左侧为标题和简介，四条要点向右延伸，并以同心数值圆作为视觉焦点。";

const BulletSchema = z.object({
  text: z.string().min(12).max(46).meta({
    description: "行左侧显示的要点文本。",
  }),
});

export const Schema = z.object({
  title: z.string().min(4).max(22).default("市场机会").meta({
    description: "左上角显示的主标题。",
  }),
  subtitle: z.string().min(40).max(110).default(
    "洞察市场趋势与客户需求，识别高潜力增长空间，并制定清晰可执行的业务策略。"
  ).meta({
    description: "主标题下方的说明文字。",
  }),
  bullets: z
    .array(BulletSchema)
    .min(4)
    .max(4)
    .default([
      { text: "目标市场规模持续扩大" },
      { text: "核心客户需求日益明确" },
      { text: "差异化优势逐步形成" },
      { text: "增长路径清晰可执行" },
    ])
    .meta({
      description: "左侧显示的四个项目符号条目。",
    }),
  values: z
    .array(z.string().min(2).max(6))
    .min(4)
    .max(4)
    .default(["$33", "$20", "$120", "$200"])
    .meta({
      description: "从外圈到内圈显示的四个数值。",
    }),
});

export type SchemaType = z.infer<typeof Schema>;


const COLORS = [
  "var(--graph-0,#5f7f79)",
  "var(--graph-1,#1f5a4f)",
  "var(--graph-2,#0d4f43)",
  "var(--graph-3,#06463d)",
];

const MarketOpportunitySlide = ({ data }: { data: Partial<SchemaType> }) => {
  const { title, subtitle, bullets, values } = data;

  return (
    <div
      className="relative h-[720px] w-[1280px] overflow-hidden rounded-[24px]"
      style={{
        backgroundColor: "var(--background-color,#DAE1DE)",
        fontFamily: "var(--body-font-family,'Bricolage Grotesque')",
      }}
    >
      <div className="px-[56px] pt-[72px]">
        <h2
          className="text-[80px] font-semibold leading-[108.4%] tracking-[-2.419px] text-[#15342D]"
          style={{ color: "var(--primary-color,#15342D)" }}
        >
          {title}
        </h2>
        <p
          className="mt-[20px] w-[730px] text-[24px] font-normal  text-[#15342DCC]"
          style={{ color: "var(--background-text,#15342DCC)" }}
        >
          {subtitle}
        </p>
      </div>

      <div className="absolute left-[56px] top-[368px] space-y-[42px]">
        {bullets?.map((bullet, index) => (
          <div key={index} className="relative flex items-center">
            <span
              className="mr-[14px] h-[14px] w-[14px] rounded-full bg-[#0a4a3f]"
              style={{ backgroundColor: "var(--graph-0,#0a4a3f)" }}
            />
            <p
              className="w-[640px] text-[24px] font-normal  text-[#15342DCC]"
              style={{ color: "var(--background-text,#15342DCC)" }}
            >
              {bullet.text}
            </p>
            <span
              className="ml-[8px] h-[2px] w-[80px] bg-[#8ea8a5]"
              style={{ backgroundColor: "var(--stroke,#8ea8a5)" }}
            />
            <span
              className="h-[6px] w-[6px] rounded-full bg-[#edf2f1]"
              style={{ backgroundColor: "var(--primary-text,#edf2f1)" }}
            />
          </div>
        ))}
      </div>

      <div className="absolute bottom-[58px] right-[48px] h-[474px] w-[474px]">
        {values?.map((value, index) => (
          <div
            key={index}
            className="absolute rounded-full"
            style={{
              width: 237 + (index * 50),
              height: 237 + (index * 50),
              bottom: 0,
              right: 0,
              backgroundColor: COLORS[index],
            }}
          >
            <p
              className="pt-[24px] text-center text-[24px] font-normal  text-white"
              style={{ color: "var(--primary-text,#ffffff)" }}
            >
              {value}
            </p>
          </div>
        ))}
      </div>
    </div>
  );
};

export default MarketOpportunitySlide;
