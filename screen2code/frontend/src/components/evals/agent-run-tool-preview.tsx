import { Light as SyntaxHighlighterBase } from "react-syntax-highlighter";
import html from "react-syntax-highlighter/dist/esm/languages/hljs/xml";
import { vs2015 } from "react-syntax-highlighter/dist/esm/styles/hljs";

import { LightboxImage } from "./image-lightbox";

SyntaxHighlighterBase.registerLanguage("html", html);
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const SyntaxHighlighter = SyntaxHighlighterBase as any;

function getRecord(value: unknown): Record<string, unknown> | null {
  return value && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : null;
}

function getArray(value: unknown, field: string): unknown[] | null {
  const record = getRecord(value);
  const fieldValue = record?.[field];
  return Array.isArray(fieldValue) ? fieldValue : null;
}

function getString(value: unknown, field: string): string | null {
  const record = getRecord(value);
  const fieldValue = record?.[field];
  return typeof fieldValue === "string" ? fieldValue : null;
}

function CompactJson({ data }: { data: unknown }) {
  let json = "";
  try {
    json = JSON.stringify(data, null, 2);
  } catch {
    json = String(data);
  }
  if (json.length > 2000) json = json.slice(0, 2000) + "…";
  return (
    <pre className="overflow-x-auto whitespace-pre-wrap break-all rounded-md bg-zinc-950 p-2 text-xs text-zinc-300">
      {json}
    </pre>
  );
}

function FieldLabel({ children }: { children: React.ReactNode }) {
  return (
    <div className="mb-1 text-xs text-zinc-500">{children}</div>
  );
}

function UrlChip({ url }: { url: string }) {
  return (
    <div className="break-all rounded bg-zinc-950 p-2 font-mono text-xs text-zinc-400">
      {url}
    </div>
  );
}

function MissingBox({ label = "缺失" }: { label?: string }) {
  return (
    <div className="flex h-20 w-20 items-center justify-center rounded bg-zinc-800 text-xs text-zinc-500">
      {label}
    </div>
  );
}

// Small thumbnail; click opens the shared zoom/pan lightbox.
function Img({ src, alt }: { src: string; alt: string }) {
  return <LightboxImage src={src} alt={alt} />;
}

/**
 * Human-readable tool previews mirroring the in-app AgentActivity views
 * (edit_images shows main/edited images, screenshot_preview shows the captured
 * screenshots, etc.), restyled for the dark eval theme. `args` is the full
 * recorded tool input; `summary` is the compact result the app itself renders.
 */
function ToolPreview({
  name,
  args,
  summary,
}: {
  name: string;
  args: unknown;
  summary: unknown;
}) {
  const error = getString(summary, "error");
  if (error) {
    return (
      <div className="rounded-md border border-red-900 bg-red-950/40 p-3">
        <div className="text-xs uppercase tracking-wide text-red-400">

          错误
        </div>
        <div className="mt-1 text-sm text-red-200">{error}</div>
        {args !== undefined && args !== null && (
          <div className="mt-2">
            <FieldLabel>输入</FieldLabel>
            <CompactJson data={args} />
          </div>
        )}
      </div>
    );
  }

  if (name === "create_file") {
    const content = getString(args, "content");
    const path = getString(args, "path");
    if (content) {
      return (
        <div>
          {path && <FieldLabel>{path}</FieldLabel>}
          <div className="max-h-80 overflow-auto rounded-md">
            <SyntaxHighlighter
              language="html"
              style={vs2015}
              customStyle={{
                margin: 0,
                padding: "0.5rem",
                fontSize: "0.75rem",
                borderRadius: "0.375rem",
              }}
              wrapLongLines
            >
              {content}
            </SyntaxHighlighter>
          </div>
        </div>
      );
    }
  }

  if (name === "edit_file") {
    const edits = getArray(summary, "edits");
    if (edits) {
      return (
        <div className="space-y-2">
          {edits.map((edit, index) => {
            const oldText = getString(edit, "old_text") ?? "";
            const newText = getString(edit, "new_text") ?? "";
            const replaced = getRecord(edit)?.replaced;
            return (
              <div
                key={`${oldText.slice(0, 40)}-${index}`}
                className="rounded-md border border-zinc-800 bg-zinc-950/60 p-3"
              >
                <div className="text-xs uppercase tracking-wide text-zinc-500">

                  编辑 {index + 1}
                </div>
                <div className="mt-2 grid gap-2">
                  <div>
                    <FieldLabel>原内容</FieldLabel>
                    <div className="break-all rounded bg-red-950/40 p-2 font-mono text-xs text-red-200">
                      {oldText}
                    </div>
                  </div>
                  <div>
                    <FieldLabel>新内容</FieldLabel>
                    <div className="break-all rounded bg-emerald-950/40 p-2 font-mono text-xs text-emerald-200">
                      {newText}
                    </div>
                  </div>
                </div>
                {replaced !== undefined && (
                  <div className="mt-2 text-xs text-zinc-500">

                    已替换 {String(replaced)}  次
                  </div>
                )}
              </div>
            );
          })}
        </div>
      );
    }
  }

  if (name === "generate_images") {
    const images = getArray(summary, "images");
    if (images) {
      return (
        <div className="divide-y divide-zinc-800">
          {images.map((item, index) => {
            const url = getString(item, "url");
            const prompt = getString(item, "prompt");
            return (
              <div key={`${prompt}-${index}`} className="flex items-center gap-3 py-1.5">
                <div className="shrink-0">
                  {url ? (
                    <Img src={url} alt={prompt || `生成的图片${index + 1}`} />
                  ) : (
                    <MissingBox label="失败" />
                  )}
                </div>
                <div className="min-w-0 flex-1 text-xs text-zinc-400">
                  {prompt}
                </div>
              </div>
            );
          })}
        </div>
      );
    }
  }

  if (name === "remove_backgrounds") {
    const images = getArray(summary, "images");
    if (images) {
      return (
        <div className="divide-y divide-zinc-800">
          {images.map((item, index) => {
            const before = getString(item, "image_url");
            const after = getString(item, "result_url");
            return (
              <div key={`${before}-${index}`} className="flex gap-4 py-1.5">
                <div>
                  <FieldLabel>修改前</FieldLabel>
                  {before ? (
                    <Img src={before} alt={`原始图片${index + 1}`} />
                  ) : (
                    <MissingBox />
                  )}
                </div>
                <div>
                  <FieldLabel>修改后</FieldLabel>
                  {after ? (
                    <Img src={after} alt="已移除背景" />
                  ) : (
                    <MissingBox label="失败" />
                  )}
                </div>
              </div>
            );
          })}
        </div>
      );
    }
  }

  if (name === "edit_images") {
    const resultImages = getArray(summary, "images") ?? [];
    const requestedEdits = getArray(args, "edits") ?? [];
    const edits = resultImages.length > 0 ? resultImages : requestedEdits;
    return (
      <div className="divide-y divide-zinc-800">
        {edits.map((item, index) => {
          const requestedEdit = requestedEdits[index];
          const prompt =
            getString(item, "prompt") ?? getString(requestedEdit, "prompt");
          const imageUrls =
            getArray(item, "image_urls") ??
            getArray(requestedEdit, "image_urls") ??
            [];
          const mainUrl =
            typeof imageUrls[0] === "string" ? imageUrls[0] : null;
          const resultUrl = getString(item, "result_url");
          const aspectRatio =
            getString(item, "aspect_ratio") ??
            getString(requestedEdit, "aspect_ratio") ??
            "match_input_image";
          const itemError = getString(item, "error");
          const referenceUrls = imageUrls
            .slice(1)
            .filter((url): url is string => typeof url === "string");
          return (
            <div key={`${prompt}-${index}`} className="space-y-3 py-3">
              <div className="flex items-center justify-between gap-3">
                <div className="text-xs font-semibold text-zinc-300">

                  编辑 {index + 1}
                </div>
                <div className="rounded-full bg-zinc-800 px-2 py-0.5 font-mono text-[10px] text-zinc-400">
                  {aspectRatio}
                </div>
              </div>
              {prompt && (
                <div>
                  <FieldLabel>提示词</FieldLabel>
                  <p className="whitespace-pre-wrap break-words text-xs text-zinc-300">
                    {prompt}
                  </p>
                </div>
              )}
              <div className="flex flex-wrap gap-4">
                <div>
                  <FieldLabel>主图</FieldLabel>
                  {mainUrl ? (
                    <Img src={mainUrl} alt={`主图${index + 1}`} />
                  ) : (
                    <MissingBox />
                  )}
                </div>
                <div>
                  <FieldLabel>编辑后的图片</FieldLabel>
                  {resultUrl ? (
                    <Img src={resultUrl} alt={`编辑后的图片${index + 1}`} />
                  ) : (
                    <MissingBox label="失败" />
                  )}
                </div>
                {referenceUrls.length > 0 && (
                  <div>
                    <FieldLabel>参考图片</FieldLabel>
                    <div className="flex flex-wrap gap-2">
                      {referenceUrls.map((url, referenceIndex) => (
                        <Img
                          key={`${url}-${referenceIndex}`}
                          src={url}
                          alt={`参考图${referenceIndex + 1}`}
                        />
                      ))}
                    </div>
                  </div>
                )}
              </div>
              {itemError && (
                <div className="rounded bg-red-950/40 p-2 text-xs text-red-200">
                  {itemError}
                </div>
              )}
              {resultUrl && (
                <div>
                  <FieldLabel>结果地址</FieldLabel>
                  <UrlChip url={resultUrl} />
                </div>
              )}
            </div>
          );
        })}
      </div>
    );
  }

  if (name === "save_assets") {
    const images = getArray(summary, "images");
    if (images) {
      return (
        <div className="divide-y divide-zinc-800">
          {images.map((item, index) => {
            const publicUrl = getString(item, "public_url");
            return (
              <div
                key={`${publicUrl}-${index}`}
                className="flex items-center gap-3 py-1.5"
              >
                <div className="shrink-0">
                  {publicUrl ? (
                    <Img src={publicUrl} alt={`已保存素材${index + 1}`} />
                  ) : (
                    <MissingBox label="失败" />
                  )}
                </div>
                <div className="min-w-0 flex-1">
                  <FieldLabel>永久地址</FieldLabel>
                  {publicUrl && <UrlChip url={publicUrl} />}
                </div>
              </div>
            );
          })}
        </div>
      );
    }
  }

  if (name === "extract_assets") {
    const assets = getArray(summary, "assets");
    if (assets) {
      return (
        <div className="divide-y divide-zinc-800">
          {assets.map((asset, index) => {
            const description =
              getString(asset, "description") ?? `素材${index + 1}`;
            const previewUrl =
              getString(asset, "public_url") ?? getString(asset, "data_url");
            const box = getArray(asset, "box_2d");
            const status = getString(asset, "status") ?? (previewUrl ? "ok" : "missing");
            return (
              <div
                key={`${description}-${index}`}
                className="flex items-center gap-3 py-1.5"
              >
                <div className="shrink-0">
                  {previewUrl ? (
                    <Img src={previewUrl} alt={description} />
                  ) : (
                    <MissingBox />
                  )}
                </div>
                <div className="min-w-0 flex-1 text-xs text-zinc-300">
                  {description}
                  <span className="ml-2 font-mono text-[11px] text-zinc-500">
                    {status}
                    {box && ` · [${box.join(", ")}]`}
                  </span>
                </div>
              </div>
            );
          })}
        </div>
      );
    }
  }

  if (name === "screenshot_preview") {
    const screenshots = getArray(summary, "screenshots") ?? [];
    return (
      <div className="grid gap-3 py-1 sm:grid-cols-2">
        {(["desktop", "mobile"] as const).map((viewport) => {
          const screenshot = screenshots.find(
            (item) => getString(item, "viewport") === viewport
          );
          const imageUrl = getString(screenshot, "image_url");
          return (
            <div key={viewport}>
              <FieldLabel>
                <span>{viewport === "desktop" ? "桌面端" : "移动端"}</span>
              </FieldLabel>
              {imageUrl ? (
                <div className="max-h-48 overflow-y-auto rounded border border-zinc-800">
                  <LightboxImage
                    src={imageUrl}
                    alt={`${viewport === "desktop" ? "桌面端" : "移动端"}预览截图`}
                    className="w-full"
                  />
                </div>
              ) : (
                <MissingBox />
              )}
            </div>
          );
        })}
      </div>
    );
  }

  // Unknown tools (retrieve_option, future additions): compact JSON.
  return (
    <div className="space-y-3">
      {args !== undefined && args !== null && (
        <div>
          <FieldLabel>输入</FieldLabel>
          <CompactJson data={args} />
        </div>
      )}
      {summary !== undefined && summary !== null && (
        <div>
          <FieldLabel>输出</FieldLabel>
          <CompactJson data={summary} />
        </div>
      )}
    </div>
  );
}

export default ToolPreview;
