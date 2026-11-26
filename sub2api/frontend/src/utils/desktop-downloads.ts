export interface WindowsDesktopClientDownload {
  labelKey: string
  url: string
}

export interface FrontendDownload {
  labelKey: string
  url: string
  filename: string
}

const ccLaunchRelease = 'https://github.com/aikenchen0-ctrl/cc2cx/releases/download/v3.20.7/'

export const IMAGE_SKILL_DOWNLOAD: FrontendDownload = {
  labelKey: 'keys.desktopDownloads.imageSkill',
  url: '/downloads/genimg-skill.zip',
  filename: 'genimg-skill.zip',
}

export const WINDOWS_DESKTOP_CLIENTS = {
  ccLaunch: {
    labelKey: 'keys.desktopDownloads.ccLaunch',
    url: `${ccLaunchRelease}cc-launch_3.20.7_x64-setup.exe`,
  },
} satisfies Record<string, WindowsDesktopClientDownload>

export function downloadWindowsDesktopClient(url: string) {
  const link = document.createElement('a')
  link.href = url
  link.target = '_blank'
  link.rel = 'noopener noreferrer'
  document.body.appendChild(link)
  link.click()
  link.remove()
}

export function downloadFrontendFile({ url, filename }: FrontendDownload) {
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  document.body.appendChild(link)
  link.click()
  link.remove()
}
