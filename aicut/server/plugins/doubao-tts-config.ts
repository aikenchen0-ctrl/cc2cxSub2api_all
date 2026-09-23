export interface DoubaoTtsConfigValues {
  baseUrl: string;
  appId: string;
  accessKey: string;
  resourceId: string;
}

/** Server-only Doubao TTS configuration. Secrets must come from the keystore/env. */
export class DoubaoTtsConfig {
  readonly baseUrl: string;
  readonly appId: string;
  readonly accessKey: string;
  readonly resourceId: string;

  constructor(values: Partial<DoubaoTtsConfigValues> = {}) {
    this.baseUrl = values.baseUrl?.trim() || 'https://openspeech.bytedance.com';
    this.appId = values.appId?.trim() || '';
    this.accessKey = values.accessKey?.trim() || '';
    this.resourceId = values.resourceId?.trim() || 'seed-tts-2.0';
  }

  static fromEnv(get: (name: string) => string | undefined): DoubaoTtsConfig {
    return new DoubaoTtsConfig({
      baseUrl: get('DOUBAO_TTS_BASE_URL'),
      appId: get('DOUBAO_TTS_APP_ID'),
      accessKey: get('DOUBAO_TTS_ACCESS_KEY') || get('DOUBAO_TTS_ACCESS_TOKEN'),
      resourceId: get('DOUBAO_TTS_RESOURCE_ID'),
    });
  }

  isConfigured(): boolean {
    return Boolean(this.appId && this.accessKey);
  }
}
