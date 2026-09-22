import { requireAdminSession } from "@/utils/serverAuth";
import AdminPanel from "./AdminPanel";

export const metadata = {
  title: "管理 | 永恒PPT",
};

export default async function AdminPage() {
  await requireAdminSession();
  return <AdminPanel />;
}
