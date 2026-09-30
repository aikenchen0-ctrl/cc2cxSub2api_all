export type DownloadOption = {
  key: string
  title: string
  description: string
  url: string
  platforms: string[]
}

export const ccswitchVersion = '3.20.0'

const release = `https://github.com/aikenchen0-ctrl/cc2cx/releases/download/v${ccswitchVersion}/`

export const downloadOptions: DownloadOption[] = [
  {
    key: 'windows-exe',
    title: 'Windows 安装程序（EXE）',
    description: '适用于大多数 Windows 设备，推荐普通用户下载。',
    url: `${release}cc-launch_${ccswitchVersion}_x64-setup.exe`,
    platforms: ['windows'],
  },
  {
    key: 'windows-msi',
    title: 'Windows 安装包（MSI）',
    description: '适用于企业部署或需要 MSI 安装格式的 Windows 设备。',
    url: `${release}cc-launch_${ccswitchVersion}_x64_en-US.msi`,
    platforms: ['windows'],
  },
  {
    key: 'mac-arm',
    title: 'macOS Apple 芯片版',
    description: '适用于搭载 Apple M 系列芯片的 Mac。',
    url: `${release}cc-launch_${ccswitchVersion}_aarch64.dmg`,
    platforms: ['mac-arm'],
  },
  {
    key: 'mac-intel',
    title: 'macOS Intel 版',
    description: '适用于搭载 Intel 处理器的 Mac。',
    url: `${release}cc-launch_${ccswitchVersion}_x64.dmg`,
    platforms: ['mac-intel'],
  },
]

export function detectPlatform(userAgent = typeof navigator === 'undefined' ? '' : navigator.userAgent): string {
  const value = userAgent.toLowerCase()
  if (value.includes('windows')) return 'windows'
  if (value.includes('macintosh') || value.includes('mac os')) {
    return value.includes('arm') || value.includes('aarch64') ? 'mac-arm' : 'mac-intel'
  }
  return 'other'
}

export function recommendDownload(userAgent?: string): DownloadOption & { platform: string } {
  const platform = detectPlatform(userAgent)
  const option = downloadOptions.find((item) => item.platforms.includes(platform)) || downloadOptions[0]
  return { ...option, platform }
}
