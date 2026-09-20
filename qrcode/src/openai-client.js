import { qrcodeRequestHeaders } from "./sso.js";

export async function callResponsesApi({
  endpoint,
  apiKey,
  payload,
  userId = "",
  headers: requestHeaders = null,
  fetchImpl = fetch
}) {
  const headers = requestHeaders || qrcodeRequestHeaders(userId, apiKey);
  const response = await fetchImpl(endpoint, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      ...headers
    },
    body: JSON.stringify(payload)
  });

  if (!response.ok) {
    const errorText = await response.text();
    throw new Error(`模型调用失败: ${response.status} ${errorText}`);
  }

  const data = await response.json();
  const outputText =
    data.output_text ??
    data.output?.flatMap((item) => item.content ?? [])
      ?.filter((item) => item.type === "output_text")
      ?.map((item) => item.text)
      ?.join("\n") ??
    "";

  return {
    raw: data,
    outputText
  };
}
