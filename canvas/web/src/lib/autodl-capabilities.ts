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
        imageMin: images.filter((key) => rules[key].required).length,
        audioMax: audios.length,
        audioMin: audios.filter((key) => rules[key].required).length,
        videoMax: videos.length,
        videoMin: videos.filter((key) => rules[key].required).length,
        firstFrame: Boolean(rules.first_frame),
        firstFrameRequired: Boolean(rules.first_frame?.required),
        lastFrame: Boolean(rules.last_frame),
        lastFrameRequired: Boolean(rules.last_frame?.required),
        duration: rules.duration || rules.audio_duration,
        resolution: rules.resolution,
    };
}
