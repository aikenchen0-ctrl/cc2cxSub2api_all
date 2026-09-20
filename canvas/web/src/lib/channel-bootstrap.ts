const parameterNames = ["baseUrl", "baseurl", "apiKey", "apikey"];

// Prefer Sub2API's fragment handoff; scrub both forms before async work.
export function readChannelBootstrap(href: string) {
    const url = new URL(href);
    const fragment = new URLSearchParams(url.hash.slice(1));
    const value = (names: string[]) => {
        for (const params of [fragment, url.searchParams]) {
            for (const name of names) if (params.has(name)) return params.get(name) || "";
        }
        return "";
    };
    const baseUrl = value(["baseUrl", "baseurl"]);
    if (![fragment, url.searchParams].some((params) => parameterNames.some((name) => params.has(name)))) return null;
    const hasFragmentConfig = parameterNames.some((name) => fragment.has(name));
    for (const name of parameterNames) {
        url.searchParams.delete(name);
        fragment.delete(name);
    }
    if (hasFragmentConfig) url.hash = fragment.toString();
    // API keys used to be passed through the URL. Keep scrubbing those legacy
    // parameters, but never expose or return them to the application.
    return { baseUrl, cleanUrl: url.pathname + url.search + url.hash };
}
