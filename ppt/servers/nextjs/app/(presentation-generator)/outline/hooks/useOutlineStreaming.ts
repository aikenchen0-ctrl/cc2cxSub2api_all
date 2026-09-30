import { useEffect, useRef, useState } from "react";
import { useDispatch, useSelector } from "react-redux";
import { notify } from "@/components/ui/sonner";
import { setOutlines } from "@/store/slices/presentationGeneration";
import { jsonrepair } from "jsonrepair";
import { RootState } from "@/store/store";
import { getApiUrl } from "@/utils/api";
import { limitOutlines } from "@/utils/presentationLimits";
import {
  isChatGptAuthRequiredMessage,
  requestChatGptReauth,
} from "@/utils/chatgptAuth";

const MAX_STREAM_RETRIES = 4;
const STREAM_RETRY_DELAY_MS = 1_000;
const DEFAULT_STATUS_MESSAGE = "Preparing your presentation outline";

export const useOutlineStreaming = (
  presentationId: string | null,
  enabled = true
) => {
  const dispatch = useDispatch();
  const { outlines } = useSelector(
    (state: RootState) => state.presentationGeneration
  );
  const [isStreaming, setIsStreaming] = useState(false);
  const [isLoading, setIsLoading] = useState(false);
  const [activeSlideIndex, setActiveSlideIndex] = useState<number | null>(null);
  const [highestActiveIndex, setHighestActiveIndex] = useState<number>(-1);
  const [statusMessage, setStatusMessage] = useState(DEFAULT_STATUS_MESSAGE);
  const [error, setError] = useState<string | null>(null);
  const [attempt, setAttempt] = useState(0);
  const retry = () => {
    outlinesRef.current = [];
    dispatch(setOutlines([]));
    setAttempt((value) => value + 1);
  };
  const outlinesRef = useRef<{ content: string }[]>(outlines);
  const prevSlidesRef = useRef<{ content: string }[]>([]);
  const activeIndexRef = useRef<number>(-1);
  const highestIndexRef = useRef<number>(-1);

  useEffect(() => {
    outlinesRef.current = outlines;
  }, [outlines]);

  useEffect(() => {
    const resetStreamingState = (message = DEFAULT_STATUS_MESSAGE) => {
      setIsStreaming(false);
      setIsLoading(false);
      setActiveSlideIndex(null);
      setHighestActiveIndex(-1);
      setStatusMessage(message);
      prevSlidesRef.current = [];
      activeIndexRef.current = -1;
      highestIndexRef.current = -1;
    };

    if (!enabled || !presentationId || outlinesRef.current.length > 0) {
      resetStreamingState();
      return;
    }

    let eventSource: EventSource | null = null;
    let accumulatedChunks = "";
    let retryCount = 0;
    let isClosed = false;
    let retryTimer: ReturnType<typeof setTimeout> | null = null;
    let completed = false;
    const retryToastId = `presentation-outline-retry-${presentationId}`;

    const closeEventSource = () => {
      if (eventSource) {
        eventSource.close();
        eventSource = null;
      }
    };

    const clearRetryTimer = () => {
      if (retryTimer) {
        clearTimeout(retryTimer);
        retryTimer = null;
      }
    };

    const scheduleRetry = (reason: string): boolean => {
      if (retryCount >= MAX_STREAM_RETRIES || isClosed) {
        if (!isClosed) {
          setError("大纲生成失败，请检查文本模型配置或稍后重试。您的需求已保留。");
          isClosed = true;
          closeEventSource();
          clearRetryTimer();
          notify.dismiss(retryToastId);
        }
        return false;
      }

      retryCount += 1;
      const retryDelay = STREAM_RETRY_DELAY_MS * retryCount;
      console.warn(
        `Outline stream retry ${retryCount}/${MAX_STREAM_RETRIES}: ${reason}`
      );

      closeEventSource();
      clearRetryTimer();
      accumulatedChunks = "";
      dispatch(setOutlines([]));
      outlinesRef.current = [];
      prevSlidesRef.current = [];
      activeIndexRef.current = -1;
      highestIndexRef.current = -1;
      setStatusMessage("Reconnecting to outline stream");
      notify.info(
        "正在重试生成",
        `大纲模型暂时没有正常响应，正在进行第 ${retryCount}/${MAX_STREAM_RETRIES} 次重试（共尝试 5 次），请稍候。`,
        { id: retryToastId, duration: retryDelay + 2500 }
      );

      retryTimer = setTimeout(() => {
        if (!isClosed) {
          openStream();
        }
      }, retryDelay);

      return true;
    };

    const openStream = () => {
      closeEventSource();
      eventSource = new EventSource(
        getApiUrl(`/api/v1/ppt/outlines/stream/${presentationId}`)
      );

      eventSource.addEventListener("response", (event) => {
        if (isClosed) return;
        let data: any;
        try {
          data = JSON.parse(event.data);
        } catch {
          if (!scheduleRetry("invalid SSE payload")) {
            resetStreamingState();
            notify.error(
              "解析流失败",
              "我已经尽力了，但还是没有生成成功。请稍后再试。"
            );
          }
          return;
        }

        switch (data.type) {
          case "status":
            if (data.status) {
              setStatusMessage(data.status);
            }
            break;

          case "chunk":
            accumulatedChunks += data.chunk;
            try {
              const repairedJson = jsonrepair(accumulatedChunks);
              const partialData = JSON.parse(repairedJson);

              if (partialData.slides) {
                const nextSlides: { content: string }[] =
                  limitOutlines(partialData.slides || []);
                try {
                  const prev = prevSlidesRef.current || [];
                  let changedIndex: number | null = null;
                  const maxLen = Math.max(prev.length, nextSlides.length);
                  for (let i = 0; i < maxLen; i++) {
                    const prevContent = prev[i]?.content;
                    const nextContent = nextSlides[i]?.content;
                    if (nextContent !== prevContent) {
                      changedIndex = i;
                    }
                  }
                  const prevActive = activeIndexRef.current;
                  let nextActive = changedIndex ?? prevActive;
                  if (nextActive < prevActive) {
                    nextActive = prevActive;
                  }
                  activeIndexRef.current = nextActive;
                  setActiveSlideIndex(nextActive);

                  if (nextActive > highestIndexRef.current) {
                    highestIndexRef.current = nextActive;
                    setHighestActiveIndex(nextActive);
                  }
                } catch {}

                prevSlidesRef.current = nextSlides;
                dispatch(setOutlines(nextSlides));
                setIsLoading(false);
              }
            } catch {
              // JSON is not complete yet, so keep accumulating chunks.
            }
            break;

          case "complete":
            try {
              const outlinesData: { content: string }[] =
                limitOutlines(data.presentation.outlines.slides);
              if (!outlinesData.length || outlinesData.some((slide) => !slide.content?.trim())) throw new Error("empty outline");
              completed = true;
              dispatch(setOutlines(outlinesData));
              setIsStreaming(false);
              setIsLoading(false);
              setActiveSlideIndex(null);
              setHighestActiveIndex(-1);
              setStatusMessage("Outline ready");
              prevSlidesRef.current = outlinesData;
              activeIndexRef.current = -1;
              highestIndexRef.current = -1;
              isClosed = true;
              closeEventSource();
              clearRetryTimer();
              notify.dismiss(retryToastId);
              retryCount = 0;
            } catch {
              if (!scheduleRetry("failed to parse complete payload")) {
                resetStreamingState();
                notify.error(
                  "演示文稿生成失败",
                  "我已经尽力了，但还是没有生成成功。请稍后再试。"
                );
              }
            }
            accumulatedChunks = "";
            break;

          case "closing":
            if (!completed) {
              if (!scheduleRetry("stream closed before completion")) resetStreamingState();
              break;
            }
            resetStreamingState("Outline ready");
            isClosed = true;
            closeEventSource();
            clearRetryTimer();
            notify.dismiss(retryToastId);
            retryCount = 0;
            break;

          case "error":
            if (isChatGptAuthRequiredMessage(data.detail)) {
              setError("登录已失效，请重新登录后重试。您的需求已保留。");
              isClosed = true;
              resetStreamingState();
              closeEventSource();
              requestChatGptReauth({
                message: data.detail,
                source: "outline-stream",
              });
              break;
            }
            if (!scheduleRetry(data.detail || "server returned stream error")) {
              resetStreamingState();
              closeEventSource();
              notify.error(
                "演示文稿生成失败",
                "我已经尽力了，但还是没有生成成功。请稍后再试。"
              );
            }
            break;
        }
      });

      eventSource.onerror = () => {
        if (isClosed) return;
        if (!scheduleRetry("connection lost")) {
          resetStreamingState();
          closeEventSource();
          notify.error(
            "演示文稿生成失败",
            "我已经尽力了，但还是没有生成成功。请稍后再试。"
          );
        }
      };
    };

    setStatusMessage(DEFAULT_STATUS_MESSAGE);
    setError(null);
    setIsStreaming(true);
    setIsLoading(true);
    openStream();

    return () => {
      isClosed = true;
      closeEventSource();
      clearRetryTimer();
      notify.dismiss(retryToastId);
    };
  }, [presentationId, dispatch, enabled, attempt]);

  return {
    isStreaming,
    isLoading,
    activeSlideIndex,
    highestActiveIndex,
    statusMessage,
    error,
    retry,
  };
};
