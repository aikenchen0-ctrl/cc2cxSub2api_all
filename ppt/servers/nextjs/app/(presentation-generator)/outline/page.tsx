import React from "react";
import { Metadata } from "next";
import OutlinePage from "./components/OutlinePage";

export const metadata: Metadata = {
  title: "大纲演示文稿",
  description: "自定义并整理您的演示文稿大纲。拖放幻灯片，添加图表，轻松生成演示文稿。",
  alternates: {
    canonical: "https://presenton.ai/create"
  },
  keywords: [
    "presentation generator",
    "AI presentations",
    "data visualization",
    "automatic presentation maker",
    "professional slides",
    "data-driven presentations",
    "document to presentation",
    "presentation automation",
    "smart presentation tool",
    "business presentations"
  ]
};

const page = () => {
  return (
    <div className="relative min-h-screen" translate="no">
      <OutlinePage />
    </div>
  );
};

export default page;
