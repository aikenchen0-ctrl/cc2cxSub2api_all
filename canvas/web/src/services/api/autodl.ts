import { apiPost } from "@/services/api/request";
import { autoDLVideoWorkflow, autoDLVideoWorkflows } from "@/lib/autodl-video-catalog";

export type AutoDLInputRule = {
    type: string;
    required?: boolean;
    default?: string | number;
    min?: number;
    max?: number;
    options?: Array<{ label: string }>;
};

export type AutoDLWorkflow = {
    uuid: string;
    name: string;
    kind: "video" | "audio" | "unsupported";
    input_rules?: Record<string, AutoDLInputRule>;
};

export function fetchAutoDLWorkflows(baseUrl: string) {
    if (!baseUrl) return Promise.resolve(autoDLVideoWorkflows);
    return apiPost<AutoDLWorkflow[]>("/api/ai/autodl/workflows", { baseUrl });
}

export function fetchAutoDLWorkflow(baseUrl: string, workflowId: string) {
    if (!baseUrl) {
        const workflow = autoDLVideoWorkflow(workflowId);
        return workflow ? Promise.resolve(workflow) : Promise.reject(new Error("未支持的 AutoDL 视频工作流"));
    }
    return apiPost<AutoDLWorkflow>("/api/ai/autodl/workflows", { baseUrl, workflowId });
}
