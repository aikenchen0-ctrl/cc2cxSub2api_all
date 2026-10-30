export interface WindowsDesktopClientDownload {
  labelKey: string
  url: string
}

const infiniteCanvasRelease = 'https://github.com/tigerowo/infinite-canvas/releases/download/v0.8.2/'
const ccLaunchRelease = 'https://github.com/aikenchen0-ctrl/cc2cx/releases/download/v3.20.7/'

export const WINDOWS_DESKTOP_CLIENTS = {
  imageVideo: {
    labelKey: 'keys.desktopDownloads.imageVideo',
    url: `${infiniteCanvasRelease}Infinite.Canvas_0.8.2_x64-setup.exe`,
  },
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
