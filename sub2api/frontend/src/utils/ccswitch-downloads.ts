export type DownloadOption = { key: string; titleKey: string; descriptionKey: string; url: string; platforms: string[] }

const release = 'https://github.com/aikenchen0-ctrl/cc2cx/releases/download/v3.20.0/'

export const downloadOptions: DownloadOption[] = [
  { key: 'windows-exe', titleKey: 'downloads.windowsExe', descriptionKey: 'downloads.windowsExeDescription', url: `${release}cc-launch_3.20.0_x64-setup.exe`, platforms: ['windows'] },
  { key: 'windows-msi', titleKey: 'downloads.windowsMsi', descriptionKey: 'downloads.windowsMsiDescription', url: `${release}cc-launch_3.20.0_x64_en-US.msi`, platforms: ['windows'] },
  { key: 'mac-arm', titleKey: 'downloads.macArm', descriptionKey: 'downloads.macArmDescription', url: `${release}cc-launch_3.20.0_aarch64.dmg`, platforms: ['mac-arm'] },
  { key: 'mac-intel', titleKey: 'downloads.macIntel', descriptionKey: 'downloads.macIntelDescription', url: `${release}cc-launch_3.20.0_x64.dmg`, platforms: ['mac-intel'] },
]

export function detectPlatform(userAgent = typeof navigator === 'undefined' ? '' : navigator.userAgent) {
  const value = userAgent.toLowerCase()
  if (value.includes('windows')) return 'windows'
  if (value.includes('macintosh') || value.includes('mac os')) return value.includes('arm') || value.includes('aarch64') ? 'mac-arm' : 'mac-intel'
  return 'other'
}

export function recommendDownload(userAgent?: string) {
  const platform = detectPlatform(userAgent)
  const option = downloadOptions.find((item) => item.platforms.includes(platform)) || downloadOptions[0]
  return { ...option, platform }
}
