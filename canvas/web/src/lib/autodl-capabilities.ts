import type { AutoDLWorkflow } from "../services/api/autodl";

export function getAutoDLCapabilities(workflow?: AutoDLWorkflow) {
    if (!workflow?.input_rules || workflow.kind === "unsupported") return undefined;
    const rules = workflow.input_rules;
    const images = Object.keys(rules).filter((key) => /^ref_image(?:_\d+)?$/.test(key));
    const audios = Object.keys(rules).filter((key) => /^ref_audio(?:_\d+)?$/.test(key));
    const videos = Object.keys(rules).filter((key) => /^ref_video(?:_\d+)?$/.test(key));
    return {
        promptRequired: Boolean(rules.prompt?.required),
        imageMax: images.length,
        audioMax: audios.length,
        videoMax: videos.length,
        firstFrame: Boolean(rules.first_frame),
        lastFrame: Boolean(rules.last_frame),
        duration: rules.duration || rules.audio_duration,
        resolution: rules.resolution,
    };
}
