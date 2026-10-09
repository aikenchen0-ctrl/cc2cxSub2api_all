import { Link } from "react-router-dom";

function EvalNavigation() {
  return (
    <div className="flex justify-between items-center w-full py-3 px-4 bg-zinc-900 text-white">
      <div className="flex items-center space-x-4">
        <Link
          to="/evals"
          className="font-medium hover:text-blue-300 transition-colors"
        >

          评测首页
        </Link>

        <div className="text-gray-500">|</div>

        <Link
          to="/evals/run"
          className="hover:text-blue-300 transition-colors"
        >

          任务
        </Link>

        <Link
          to="/evals/best-of-n"
          className="hover:text-blue-300 transition-colors"
        >

          多方案对比
        </Link>

        <Link
          to="/evals/openai-input-compare"
          className="hover:text-blue-300 transition-colors"
        >

          输入对比
        </Link>

        <Link
          to="/evals/prompt-reports"
          className="hover:text-blue-300 transition-colors"
        >

          提示词报告
        </Link>

        <Link
          to="/evals/agent-runs"
          className="hover:text-blue-300 transition-colors"
        >

          智能体任务
        </Link>

        <Link
          to="/evals/sessions"
          className="hover:text-blue-300 transition-colors"
        >

          会话
        </Link>

        <Link
          to="/evals/compare"
          className="hover:text-blue-300 transition-colors"
        >

          对比
        </Link>
      </div>

      <Link
        to="/"
        className="text-sm text-gray-300 hover:text-white transition-colors"
      >

        ← 返回应用
      </Link>
    </div>
  );
}

export default EvalNavigation;
