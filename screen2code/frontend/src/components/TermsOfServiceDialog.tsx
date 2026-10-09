import React from "react";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogContent,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "./ui/alert-dialog";
import { Input } from "./ui/input";
import toast from "react-hot-toast";
import { PICO_BACKEND_FORM_SECRET } from "../config";

const LOGOS = ["microsoft", "amazon", "mit", "stanford", "bytedance", "baidu"];

const TermsOfServiceDialog: React.FC<{
  open: boolean;
  onOpenChange: (open: boolean) => void;
}> = ({ open, onOpenChange }) => {
  const [email, setEmail] = React.useState("");

  const onSubscribe = async () => {
    await fetch("https://backend.buildpicoapps.com/form", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ email, secret: PICO_BACKEND_FORM_SECRET }),
    });
  };

  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle className="mb-2 text-xl">

            输入邮箱开始使用
          </AlertDialogTitle>
        </AlertDialogHeader>

        <div className="mb-2">
          <Input
            placeholder="邮箱"
            value={email}
            onChange={(e) => {
              setEmail(e.target.value);
            }}
          />
        </div>
        <div className="flex flex-col space-y-3 text-sm">
          <p>

            提交邮箱即表示同意接收产品更新，并接受{" "}
            <a
              href="/legal/terms-of-service.html"
              target="_blank"
              className="underline"
            >

              服务条款
            </a>
            .{" "}
          </p>

          <p>
            {" "}

            希望在本地运行？本项目已开源。{" "}
            <a
              href="https://github.com/abi/screenshot-to-code"
              target="_blank"
              className="underline"
            >

              可从 GitHub 下载代码开始使用。
            </a>
          </p>
        </div>

        <AlertDialogFooter>
          <AlertDialogAction
            onClick={(e) => {
              if (!email.trim() || !email.trim().includes("@")) {
                e.preventDefault();
                toast.error("请输入邮箱");
              } else {
                onSubscribe();
              }
            }}
          >

            同意并继续
          </AlertDialogAction>
        </AlertDialogFooter>

        {/* Logos */}
        <div>
          <div
            className="mx-auto grid max-w-lg items-center gap-x-2
          gap-y-10 sm:max-w-xl grid-cols-6 lg:mx-0 lg:max-w-none mt-10"
          >
            {LOGOS.map((companyName) => (
              <img
                key={companyName}
                className="col-span-1 max-h-12 w-full object-contain grayscale opacity-50 hover:opacity-100"
                src={`/logos/${companyName}.png`}
                alt={companyName}
                width={120}
                height={48}
              />
            ))}
          </div>
          <div className="text-gray-500 text-xs mt-4 text-center">

            设计师和工程师使用 AI 一键复刻网站快速构建界面。
          </div>
        </div>
      </AlertDialogContent>
    </AlertDialog>
  );
};

export default TermsOfServiceDialog;
