jest.mock("../config", () => ({ HTTP_BACKEND_URL: "http://localhost:5173" }));

import { createHttpDesignSystemsRequest } from "./design-systems";

describe("design systems HTTP requests", () => {
  it("identifies an expired or missing login session", async () => {
    const fetcher = jest.fn().mockResolvedValue({ ok: false, status: 401 });
    const request = createHttpDesignSystemsRequest(undefined, fetcher);
    await expect(request("/api/design-systems")).rejects.toMatchObject({
      status: 401, message: "请从主站菜单登录后进入",
    });
  });
  it("carries the session cookie when loading and editing design systems", async () => {
    const fetcher = jest.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => [],
    });
    const request = createHttpDesignSystemsRequest(undefined, fetcher);

    await request("/api/design-systems");
    await request("/api/design-systems/example", {
      method: "PATCH",
      body: { name: "Updated" },
    });

    expect(fetcher).toHaveBeenNthCalledWith(
      1,
      "http://localhost:5173/api/design-systems",
      expect.objectContaining({ method: "GET", credentials: "include" })
    );
    expect(fetcher).toHaveBeenNthCalledWith(
      2,
      "http://localhost:5173/api/design-systems/example",
      expect.objectContaining({
        method: "PATCH",
        credentials: "include",
        body: JSON.stringify({ name: "Updated" }),
      })
    );
  });

  it("keeps backend errors visible and handles empty delete responses", async () => {
    const fetcher = jest.fn()
      .mockResolvedValueOnce({ ok: false, status: 500, text: async () => "Storage unavailable" })
      .mockResolvedValueOnce({ ok: true, status: 204 });
    const request = createHttpDesignSystemsRequest(undefined, fetcher);

    await expect(request("/api/design-systems")).rejects.toThrow("Storage unavailable");
    await expect(request("/api/design-systems/example", { method: "DELETE" })).resolves.toBeUndefined();
  });
});
