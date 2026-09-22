import type { Metadata } from "next";

import CommunityPage from "./components/CommunityPage";

export const metadata: Metadata = {
  title: "社区 | 永恒PPT",
  description: "Explore community presentation designs and prompts.",
};

export default function Page() {
  return <CommunityPage />;
}
