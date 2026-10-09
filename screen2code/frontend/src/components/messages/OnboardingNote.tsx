export function OnboardingNote() {
  return (
    <div className="flex flex-col space-y-4 bg-green-700 p-2 rounded text-stone-200 text-sm">
      <span>

        使用 AI 一键复刻网站，{" "}
        <a
          className="inline underline hover:opacity-70"
          href="https://buy.stripe.com/8wM6sre70gBW1nqaEE"
          target="_blank"
        >

          请确保账户余额充足
        </a>{" "}

        ，从主站菜单进入即可使用。{" "}
        <a
          href="https://github.com/abi/screenshot-to-code/blob/main/Troubleshooting.md"
          className="inline underline hover:opacity-70"
          target="_blank"
        >

          查看使用说明。
        </a>{" "}

        独立运行时可在设置中配置密钥，门户模式由服务器统一调用。
      </span>
    </div>
  );
}
