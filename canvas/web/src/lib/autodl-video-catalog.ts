import workflows from "./autodl-video-workflows.json";
import type { AutoDLWorkflow } from "../services/api/autodl";

// Same public metadata snapshot as Sub2API's domain/autodl_video_workflows.json.
// Names are display-only; requests always retain the exact workflow UUID.
export const autoDLVideoWorkflows = workflows as AutoDLWorkflow[];
export function autoDLVideoWorkflow(model: string) {
    return autoDLVideoWorkflows.find((workflow) => workflow.uuid === model);
}
