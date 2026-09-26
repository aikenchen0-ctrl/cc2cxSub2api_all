import type { Metadata } from "next";

import CommunityPage from "./components/CommunityPage";

export const metadata: Metadata = {
  title: "社区 | 永恒PPT",
  description: "浏览社区演示文稿设计和生成描述。",
};

export default function Page() {
  return <CommunityPage />;
}
