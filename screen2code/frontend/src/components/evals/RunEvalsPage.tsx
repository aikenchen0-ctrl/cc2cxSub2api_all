import { useState, useEffect, useCallback, useRef } from "react";
import { Button } from "../ui/button";
import { Progress } from "../ui/progress";
import { HTTP_BACKEND_URL } from "../../config";
import { BsCheckLg, BsChevronDown, BsChevronRight } from "react-icons/bs";
import InputFileSelector from "./InputFileSelector";
import EvalNavigation from "./EvalNavigation";

interface ModelResponse {
  models: string[];
  stacks: string[];
}

interface EvalProgressEvent {
  type: "start" | "model_start" | "task_complete" | "complete" | "error";
  message?: string;
  model?: string;
  model_index?: number;
  total_models?: number;
  completed_tasks?: number;
  total_tasks?: number;
  global_completed_tasks?: number;
  global_total_tasks?: number;
  input_file?: string;
  success?: boolean;
  error?: string;
  output_files?: string[];
  diff_mode?: boolean;
  total_skipped_existing?: number;
  model_tasks?: number;
  model_skipped_existing?: number;
}

interface FailedTask {
  model: string;
  inputFile: string;
  error: string;
}

interface EvalSetSummary {
  name: string;
  display_name: string;
  image_count: number;
}

interface EvalSetImage {
  filename: string;
}

interface ActiveEvalSession {
  session_id: string;
  name: string;
  eval_set: string;
}

function RunEvalsPage() {
  const faviconFlashIntervalRef = useRef<number | null>(null);
  const [isRunning, setIsRunning] = useState(false);
  const [models, setModels] = useState<string[]>([]);
  const [stacks, setStacks] = useState<string[]>([]);
  const [selectedModels, setSelectedModels] = useState<string[]>([]);
  const [selectedStack, setSelectedStack] = useState<string>("html_tailwind");
  const [diffMode, setDiffMode] = useState(true);
  const [selectedFiles, setSelectedFiles] = useState<string[]>([]);
  const [inputSource, setInputSource] = useState<"legacy" | "set">("legacy");
  const [evalSets, setEvalSets] = useState<EvalSetSummary[]>([]);
  const [selectedSet, setSelectedSet] = useState("");
  const [setImages, setSetImages] = useState<EvalSetImage[]>([]);
  const [selectedSetFiles, setSelectedSetFiles] = useState<string[]>([]);
  const [activeSession, setActiveSession] = useState<ActiveEvalSession | null>(
    null
  );
  const [showPaths, setShowPaths] = useState<boolean>(false);
  const [completedTasks, setCompletedTasks] = useState(0);
  const [totalTasks, setTotalTasks] = useState(0);
  const [currentModel, setCurrentModel] = useState<string>("");
  const [statusMessage, setStatusMessage] = useState<string>("等待开始");
  const [lastProcessedFile, setLastProcessedFile] = useState<string>("");
  const [failedTasks, setFailedTasks] = useState(0);
  const [failedTaskDetails, setFailedTaskDetails] = useState<FailedTask[]>([]);
  const [skippedExistingTasks, setSkippedExistingTasks] = useState(0);

  useEffect(() => {
    const fetchModels = async () => {
      const response = await fetch(`${HTTP_BACKEND_URL}/models`);
      const data: ModelResponse = await response.json();
      setModels(data.models);
      setStacks(data.stacks);
    };
    fetchModels();
  }, []);

  useEffect(() => {
    const fetchEvalSets = async () => {
      try {
        const response = await fetch(`${HTTP_BACKEND_URL}/eval-sets`);
        const data: EvalSetSummary[] = await response.json();
        setEvalSets(data);
        if (data.length > 0) {
          setSelectedSet((current) => current || data[0].name);
        }
      } catch (error) {
        console.error("获取评测集失败", error);
      }
    };
    fetchEvalSets();
  }, []);

  const refreshActiveSession = useCallback(async () => {
    try {
      const response = await fetch(`${HTTP_BACKEND_URL}/eval-sessions/active`);
      setActiveSession(await response.json());
    } catch (error) {
      console.error("获取当前评测会话失败", error);
    }
  }, []);

  useEffect(() => {
    if (inputSource === "set") refreshActiveSession();
  }, [inputSource, refreshActiveSession]);

  useEffect(() => {
    if (inputSource !== "set" || !selectedSet) return;
    const fetchSetImages = async () => {
      try {
        const response = await fetch(
          `${HTTP_BACKEND_URL}/eval-sets/${encodeURIComponent(selectedSet)}`
        );
        if (!response.ok) throw new Error("请求失败");
        const data = await response.json();
        const items: EvalSetImage[] =
          data.kind === "text"
            ? data.briefs.map((b: { id: string }) => ({ filename: b.id }))
            : data.images;
        setSetImages(items);
        setSelectedSetFiles(items.map((item) => item.filename));
      } catch (error) {
        console.error("获取评测集图片失败", error);
        setSetImages([]);
        setSelectedSetFiles([]);
      }
    };
    fetchSetImages();
  }, [inputSource, selectedSet]);

  useEffect(() => {
    return () => {
      document.title = "AI一键复刻网站";
      if (faviconFlashIntervalRef.current !== null) {
        window.clearInterval(faviconFlashIntervalRef.current);
      }
    };
  }, []);

  const setFavicon = (href: string) => {
    const faviconEl = document.querySelector(
      "link[rel='icon']"
    ) as HTMLLinkElement | null;
    if (faviconEl) {
      faviconEl.href = href;
    }
  };

  const getSiteLogo = () =>
    (window as Window & { __SUB2API_SITE_LOGO__?: string }).__SUB2API_SITE_LOGO__ || "/project-icon.jpg";

  const stopFaviconFlash = () => {
    if (faviconFlashIntervalRef.current !== null) {
      window.clearInterval(faviconFlashIntervalRef.current);
      faviconFlashIntervalRef.current = null;
    }
    setFavicon(getSiteLogo());
    window.removeEventListener("visibilitychange", stopWhenTabIsVisible);
    window.removeEventListener("focus", stopWhenTabIsVisible);
  };

  const stopWhenTabIsVisible = () => {
    if (document.visibilityState === "visible" && document.hasFocus()) {
      stopFaviconFlash();
    }
  };

  const flashFaviconOnComplete = () => {
    stopFaviconFlash();
    let useAlertIcon = false;
    faviconFlashIntervalRef.current = window.setInterval(() => {
      setFavicon(useAlertIcon ? "/favicon/coding.png" : getSiteLogo());
      useAlertIcon = !useAlertIcon;
    }, 450);
    window.addEventListener("visibilitychange", stopWhenTabIsVisible);
    window.addEventListener("focus", stopWhenTabIsVisible);
  };

  const runEvals = async (filesToRun?: string[]) => {
    const updateRunningTitle = (completed: number, total: number) => {
      if (total <= 0) {
        document.title = "AI一键复刻网站 · 运行评测…";
        return;
      }
      const percent = Math.round((completed / total) * 100);
      document.title = `(${percent}%) AI一键复刻网站 · 运行评测…`;
    };

    try {
      setIsRunning(true);
      document.title = "AI一键复刻网站 · 运行评测…";
      setCompletedTasks(0);
      setTotalTasks(0);
      setCurrentModel("");
      setStatusMessage("正在准备评测…");
      setLastProcessedFile("");
      setFailedTasks(0);
      setFailedTaskDetails([]);
      setSkippedExistingTasks(0);
      stopFaviconFlash();

      const isSetRun = inputSource === "set" && selectedSet;
      const runFiles =
        filesToRun ??
        (isSetRun
          ? // Whole set = empty list so the backend resolves every image.
            selectedSetFiles.length === setImages.length
            ? []
            : selectedSetFiles
          : selectedFiles);

      const response = await fetch(`${HTTP_BACKEND_URL}/run_evals_stream`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({
          models: selectedModels,
          stack: selectedStack,
          files: runFiles,
          diff_mode: diffMode,
          set_name: isSetRun ? selectedSet : null,
        }),
      });

      if (!response.ok) {
        throw new Error("运行评测失败");
      }

      if (!response.body) {
        throw new Error("没有可用的进度数据流");
      }

      const reader = response.body.getReader();
      const decoder = new TextDecoder();
      let bufferedText = "";

      let streamDone = false;
      while (!streamDone) {
        const { value, done } = await reader.read();
        if (done) {
          streamDone = true;
          continue;
        }

        bufferedText += decoder.decode(value, { stream: true });
        const lines = bufferedText.split("\n");
        bufferedText = lines.pop() ?? "";

        for (const line of lines) {
          if (!line.trim()) continue;

          const event = JSON.parse(line) as EvalProgressEvent;

          if (event.type === "start") {
            const eventTotalTasks = event.total_tasks ?? 0;
            setTotalTasks(eventTotalTasks);
            setSkippedExistingTasks(event.total_skipped_existing ?? 0);
            setStatusMessage(
              event.diff_mode
                ? `正在开始增量评测（${event.total_skipped_existing ?? 0}个已有结果将跳过）…`
                : "正在开始评测…"
            );
            updateRunningTitle(0, eventTotalTasks);
          } else if (event.type === "model_start") {
            if (event.model) setCurrentModel(event.model);
            setStatusMessage(
              `正在评测模型${event.model_index ?? 1}/${event.total_models ?? selectedModels.length}: ${event.model ?? "未知"}${
                diffMode && (event.model_skipped_existing ?? 0) > 0
                  ? ` (${event.model_skipped_existing} skipped)`
                  : ""
              }`
            );
          } else if (event.type === "task_complete") {
            const globalCompleted = event.global_completed_tasks ?? 0;
            const globalTotal = event.global_total_tasks ?? totalTasks;
            setCompletedTasks(globalCompleted);
            if (event.input_file) setLastProcessedFile(event.input_file);
            if (event.success === false) {
              setFailedTasks((prev) => prev + 1);
              setFailedTaskDetails((prev) => [
                ...prev,
                {
                  model: event.model ?? "unknown",
                  inputFile: event.input_file ?? "unknown",
                  error: event.error ?? "未知错误",
                },
              ]);
            }
            setStatusMessage(
              event.success === false
                ? `Failed: ${event.input_file ?? "未知文件"}`
                : `Processed: ${event.input_file ?? "未知文件"}`
            );
            updateRunningTitle(globalCompleted, globalTotal);
          } else if (event.type === "complete") {
            const finalCompleted = event.completed_tasks ?? completedTasks;
            const finalTotal = event.total_tasks ?? totalTasks;
            setCompletedTasks(finalCompleted);
            setTotalTasks(finalTotal);
            setStatusMessage("评测完成");
            console.log("生成文件：", event.output_files ?? []);
            updateRunningTitle(finalCompleted, finalTotal);
          } else if (event.type === "error") {
            throw new Error(event.message ?? "评测失败");
          }
        }
      }

      document.title = "✓ AI一键复刻网站评测完成";
      flashFaviconOnComplete();
    } catch (error) {
      console.error("运行评测失败：", error);
      document.title = "❌ AI一键复刻网站评测失败";
      setStatusMessage("评测失败");
      flashFaviconOnComplete();
      setTimeout(() => {
        document.title = "AI一键复刻网站";
      }, 5000);
    } finally {
      setIsRunning(false);
    }
  };

  const handleModelToggle = (model: string) => {
    setSelectedModels((prev) => {
      if (prev.includes(model)) {
        return prev.filter((m) => m !== model);
      }
      return [...prev, model];
    });
  };

  const handleSelectAll = () => {
    setSelectedModels(models);
  };

  const handleFilesSelected = useCallback((files: string[]) => {
    setSelectedFiles(files);
  }, []);

  // Format model list for display in the summary
  const formatModelList = () => {
    if (selectedModels.length === 0) return "无";
    if (selectedModels.length === 1) return selectedModels[0];
    if (selectedModels.length <= 2) return selectedModels.join(", ");
    return `${selectedModels.slice(0, 2).join(", ")} +${selectedModels.length - 2} more`;
  };

  const canRunEvals =
    selectedModels.length > 0 &&
    (inputSource === "set"
      ? Boolean(selectedSet) && selectedSetFiles.length > 0
      : selectedFiles.length > 0);
  const sessionSetMismatch =
    inputSource === "set" &&
    activeSession !== null &&
    Boolean(selectedSet) &&
    activeSession.eval_set !== selectedSet;

  const startSessionForSelectedSet = async () => {
    try {
      const response = await fetch(`${HTTP_BACKEND_URL}/eval-sessions`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ eval_set: selectedSet }),
      });
      if (!response.ok) throw new Error("请求失败");
      await refreshActiveSession();
    } catch (error) {
      console.error("启动评测会话失败", error);
    }
  };
  const failedFilesForRerun = Array.from(
    new Set(failedTaskDetails.map((task) => task.inputFile).filter(Boolean))
  );
  const canRerunFailures =
    !isRunning &&
    selectedModels.length > 0 &&
    failedFilesForRerun.length > 0;
  const progressPercent = totalTasks > 0 ? (completedTasks / totalTasks) * 100 : 0;

  return (
    <div>
      <EvalNavigation />

      <div className="container mx-auto px-4 py-6">
        {/* Unified Header with Configuration Summary */}
        <div className="mb-6 bg-white rounded-lg border border-gray-200 shadow-sm p-4 max-w-5xl mx-auto">
          <div className="flex flex-col gap-4">
            <div className="flex flex-wrap justify-between items-center">
              <h1 className="text-2xl font-bold">运行评测</h1>

              <Button
                onClick={() => void runEvals()}
                disabled={isRunning || !canRunEvals}
                className={`min-w-[120px] ${isRunning ? "bg-blue-400" : "bg-blue-600 hover:bg-blue-700"}`}
              >
                {isRunning ? "正在运行…" : "运行评测"}
              </Button>
            </div>

            {diffMode && (
              <div className="rounded-md border border-emerald-200 bg-emerald-50 px-3 py-2 text-sm text-emerald-800">

                增量模式已启用：仅生成今日模型目录中缺少的结果，已有结果将跳过且不会覆盖。
              </div>
            )}

            <div className="grid grid-cols-1 md:grid-cols-3 gap-3 border-t border-gray-100 pt-3">
              <div className="flex flex-col">
                <span className="text-sm font-medium text-gray-700">模型</span>
                <span className="text-sm text-gray-600 font-mono">{formatModelList()}</span>
              </div>

              <div className="flex flex-col">
                <span className="text-sm font-medium text-gray-700">技术栈</span>
                <span className="text-sm text-gray-600 font-mono">{selectedStack}</span>
              </div>

              <div className="flex flex-col">
                <span className="text-sm font-medium text-gray-700">输入文件</span>
                <span className="text-sm text-gray-600">{selectedFiles.length}  已选择</span>
              </div>
            </div>

            <div
              className="flex items-center gap-1 text-xs text-gray-600 cursor-pointer mt-1 hover:bg-gray-50 inline-block rounded px-2 py-1"
              onClick={() => setShowPaths(!showPaths)}
            >
              {showPaths ? <BsChevronDown size={12} /> : <BsChevronRight size={12} />}
              <span className="font-medium">路径</span>
            </div>

            {showPaths && (
              <div className="grid grid-cols-1 md:grid-cols-2 gap-3 text-xs text-gray-600 mt-2 bg-gray-50 p-2 rounded-md">
                <div>
                  <span className="font-medium">输入路径：</span>
                  <code className="ml-2 bg-gray-100 px-2 py-0.5 rounded">
                    backend/evals_data/inputs
                  </code>
                </div>
                <div>
                  <span className="font-medium">输出路径：</span>
                  <code className="ml-2 bg-gray-100 px-2 py-0.5 rounded">
                    backend/evals_data/results
                  </code>
                </div>
              </div>
            )}

            {(isRunning || totalTasks > 0) && (
              <div className="border-t border-gray-100 pt-3">
                <div className="flex items-center justify-between text-sm mb-2">
                  <span className="font-medium text-gray-700">{statusMessage}</span>
                  <span className="text-gray-600">
                    {completedTasks} / {totalTasks || "?"}  个任务
                  </span>
                </div>
                <Progress value={progressPercent} className="h-2 mb-2" />
                <div className="grid grid-cols-1 md:grid-cols-3 gap-2 text-xs text-gray-600">
                  <span>

                    当前模型：{" "}
                    <span className="font-mono">{currentModel || "-"}</span>
                  </span>
                  <span>

                    失败数： <span className="font-medium">{failedTasks}</span>
                  </span>
                  <span className="truncate" title={lastProcessedFile}>

                    最近文件：{" "}
                    <span className="font-mono">{lastProcessedFile || "-"}</span>
                  </span>
                </div>
                {diffMode && (
                  <div className="mt-2 text-xs text-emerald-700">

                    已跳过的结果：{" "}
                    <span className="font-medium">{skippedExistingTasks}</span>
                  </div>
                )}
                {failedTaskDetails.length > 0 && (
                  <div className="mt-3 border-t border-gray-100 pt-3">
                    <div className="flex items-center justify-between mb-2">
                      <span className="text-sm font-medium text-red-700">

                        失败（{failedTaskDetails.length})
                      </span>
                      <Button
                        variant="outline"
                        size="sm"
                        disabled={!canRerunFailures}
                        onClick={() => void runEvals(failedFilesForRerun)}
                        className="h-7 px-2 text-xs"
                      >

                        重试失败任务
                      </Button>
                    </div>
                    <div className="max-h-40 overflow-y-auto rounded-md border border-red-100 bg-red-50/40">
                      <ul className="divide-y divide-red-100">
                        {failedTaskDetails.map((task, index) => (
                          <li key={`${task.model}-${task.inputFile}-${index}`} className="px-3 py-2 text-xs">
                            <div className="font-medium text-red-800">
                              {task.inputFile}
                            </div>
                            <div className="text-red-700">

                              模型： <span className="font-mono">{task.model}</span>
                            </div>
                            <div className="text-red-700 break-words">

                              错误： {task.error}
                            </div>
                          </li>
                        ))}
                      </ul>
                    </div>
                  </div>
                )}
              </div>
            )}
          </div>
        </div>

        {/* Selection Controls */}
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6 max-w-5xl mx-auto">
          {/* Model Selection Section */}
          <div className="bg-white rounded-lg border border-gray-200 shadow-sm">
            <div className="border-b border-gray-200 px-4 py-3 bg-gray-50 rounded-t-lg">
              <h2 className="font-medium">选择模型</h2>
            </div>
            <div className="p-3">
              <div className="border rounded-md max-h-[300px] overflow-y-auto">
                <div className="grid grid-cols-1 divide-y divide-gray-100">
                  {models.map((model) => (
                    <div
                      key={model}
                      className={`flex items-center justify-between px-3 py-2 cursor-pointer hover:bg-gray-50 ${
                        selectedModels.includes(model)
                          ? "bg-blue-50"
                          : ""
                      }`}
                      onClick={() => handleModelToggle(model)}
                    >
                      <span className="text-sm truncate" title={model}>{model}</span>
                      {selectedModels.includes(model) ? (
                        <BsCheckLg className="h-3.5 w-3.5 text-blue-500 flex-shrink-0" />
                      ) : (
                        <div className="h-3.5 w-3.5 border rounded-sm flex-shrink-0" />
                      )}
                    </div>
                  ))}
                </div>
              </div>
              <div className="flex justify-between mt-2 text-xs">
                <p className="text-gray-500">

                  已选择： {selectedModels.length} / {models.length}
                </p>
                <div className="space-x-2">
                  {selectedModels.length < models.length && (
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={handleSelectAll}
                      className="text-xs h-6 px-2 text-gray-500 hover:text-gray-700"
                    >

                      全选
                    </Button>
                  )}
                  {selectedModels.length > 0 && (
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => setSelectedModels([])}
                      className="text-xs h-6 px-2 text-gray-500 hover:text-gray-700"
                    >

                      清空
                    </Button>
                  )}
                </div>
              </div>
            </div>
          </div>

          {/* Stack Selection Section */}
          <div className="bg-white rounded-lg border border-gray-200 shadow-sm">
            <div className="border-b border-gray-200 px-4 py-3 bg-gray-50 rounded-t-lg">
              <h2 className="font-medium">选择技术栈</h2>
            </div>
            <div className="p-3">
              <select
                value={selectedStack}
                onChange={(e) => setSelectedStack(e.target.value)}
                className="w-full py-2 px-3 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500"
              >
                {stacks.map((stack) => (
                  <option key={stack} value={stack}>
                    {stack}
                  </option>
                ))}
              </select>
              <label className="mt-3 flex items-center gap-2 text-sm text-gray-700 cursor-pointer">
                <input
                  type="checkbox"
                  checked={diffMode}
                  onChange={(e) => setDiffMode(e.target.checked)}
                  className="h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500"
                />
                <span>增量模式（仅生成缺少的结果，不覆盖）</span>
              </label>
            </div>
          </div>

          {/* Input Files Section */}
          <div className="bg-white rounded-lg border border-gray-200 shadow-sm lg:col-span-1 md:col-span-2">
            <div className="border-b border-gray-200 px-4 py-3 bg-gray-50 rounded-t-lg flex items-center justify-between">
              <h2 className="font-medium">选择输入文件</h2>
              <div className="flex rounded-md border border-gray-300 overflow-hidden text-xs">
                {(
                  [
                    ["legacy", "旧输入目录"],
                    ["set", "评测集"],
                  ] as const
                ).map(([source, label]) => (
                  <button
                    key={source}
                    onClick={() => setInputSource(source)}
                    className={`px-2.5 py-1 transition-colors ${
                      inputSource === source
                        ? "bg-blue-600 text-white"
                        : "bg-white text-gray-600 hover:bg-gray-100"
                    }`}
                  >
                    {label}
                  </button>
                ))}
              </div>
            </div>
            <div className="p-3">
              {inputSource === "legacy" ? (
                <InputFileSelector onFilesSelected={handleFilesSelected} />
              ) : (
                <div className="flex flex-col gap-3">
                  <select
                    value={selectedSet}
                    onChange={(e) => setSelectedSet(e.target.value)}
                    className="w-full py-2 px-3 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500"
                  >
                    {evalSets.length === 0 && (
                      <option value="">暂无评测集</option>
                    )}
                    {evalSets.map((set) => (
                      <option key={set.name} value={set.name}>
                        {set.display_name} ({set.image_count}  张图片）
                      </option>
                    ))}
                  </select>

                  {sessionSetMismatch && activeSession && (
                    <div className="rounded-md border border-amber-300 bg-amber-50 px-3 py-2 text-xs text-amber-800">

                      当前会话「{activeSession.name}」绑定评测集「
                      {activeSession.eval_set}".{" "}
                      <button
                        onClick={() => void startSessionForSelectedSet()}
                        className="font-medium underline hover:text-amber-900"
                      >

                        请为评测集「{selectedSet}"
                      </button>{" "}

                      创建新会话后再运行。
                    </div>
                  )}
                  {!sessionSetMismatch && inputSource === "set" && (
                    <div className="rounded-md border border-blue-200 bg-blue-50 px-3 py-2 text-xs text-blue-800">
                      {activeSession
                        ? `任务将归入会话「${activeSession.name}」（评测集「${activeSession.eval_set}").`
                        : "没有当前会话，将自动创建。"}
                    </div>
                  )}

                  <div className="border rounded-md max-h-[220px] overflow-y-auto">
                    <div className="grid grid-cols-1 divide-y divide-gray-100">
                      {setImages.map((image) => {
                        const checked = selectedSetFiles.includes(
                          image.filename
                        );
                        return (
                          <div
                            key={image.filename}
                            className={`flex items-center justify-between px-3 py-1.5 cursor-pointer hover:bg-gray-50 ${
                              checked ? "bg-blue-50" : ""
                            }`}
                            onClick={() =>
                              setSelectedSetFiles((previous) =>
                                checked
                                  ? previous.filter(
                                      (f) => f !== image.filename
                                    )
                                  : [...previous, image.filename]
                              )
                            }
                          >
                            <span
                              className="text-sm truncate"
                              title={image.filename}
                            >
                              {image.filename}
                            </span>
                            {checked ? (
                              <BsCheckLg className="h-3.5 w-3.5 text-blue-500 flex-shrink-0" />
                            ) : (
                              <div className="h-3.5 w-3.5 border rounded-sm flex-shrink-0" />
                            )}
                          </div>
                        );
                      })}
                    </div>
                  </div>
                  <p className="text-xs text-gray-500">

                    已选择： {selectedSetFiles.length} / {setImages.length}
                  </p>
                </div>
              )}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}

export default RunEvalsPage;
