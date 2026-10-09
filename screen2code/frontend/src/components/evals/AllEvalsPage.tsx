import { Link } from "react-router-dom";

function AllEvalsPage() {
  return (
    <div className="flex flex-col items-center justify-center min-h-screen bg-gray-50">
      <div className="max-w-md w-full space-y-8 p-8">
        <div className="flex justify-between items-center mb-4">
          <h1 className="text-3xl font-bold text-gray-900">

            评测工作台
          </h1>
          <Link
            to="/"
            className="text-sm text-gray-500 hover:text-gray-700 flex items-center"
          >

            ← 返回应用
          </Link>
        </div>
        <div className="space-y-4">
          <Link
            to="/evals/run"
            className="block w-full p-4 bg-white rounded-lg shadow hover:shadow-md transition-shadow border border-gray-200"
          >
            <h2 className="text-xl font-semibold text-gray-800">运行评测</h2>
            <p className="text-gray-600">

              同时评测多个模型
            </p>
          </Link>

          <Link
            to="/evals/best-of-n"
            className="block w-full p-4 bg-white rounded-lg shadow hover:shadow-md transition-shadow border border-gray-200"
          >
            <h2 className="text-xl font-semibold text-gray-800">多方案对比</h2>
            <p className="text-gray-600">

              并排比较多个模型的生成结果
            </p>
          </Link>

          <Link
            to="/evals/openai-input-compare"
            className="block w-full p-4 bg-white rounded-lg shadow hover:shadow-md transition-shadow border border-gray-200"
          >
            <h2 className="text-xl font-semibold text-gray-800">

              OpenAI 输入对比
            </h2>
            <p className="text-gray-600">

              定位两个请求中首个不同的输入块
            </p>
          </Link>

          <Link
            to="/evals/prompt-reports"
            className="block w-full p-4 bg-white rounded-lg shadow hover:shadow-md transition-shadow border border-gray-200"
          >
            <h2 className="text-xl font-semibold text-gray-800">

              提示词报告
            </h2>
            <p className="text-gray-600">

              查看模型请求、图片、用量和费用记录
            </p>
          </Link>
        </div>
      </div>
    </div>
  );
}

export default AllEvalsPage;
