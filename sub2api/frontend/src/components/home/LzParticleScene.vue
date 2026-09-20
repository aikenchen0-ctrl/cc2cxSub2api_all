<template>
  <canvas ref="canvasRef" class="lz-particle-canvas" aria-hidden="true"></canvas>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'

export type LzMode = 'AIFX' | 'CC2CX' | 'OPENAI' | 'DEEPSEEK'

const props = defineProps<{ activeMode: LzMode }>()
const emit = defineEmits<{ cycle: [] }>()
const canvasRef = ref<HTMLCanvasElement | null>(null)
let frameId = 0
let resizeHandler: (() => void) | undefined
let cycleTimer = 0

onMounted(() => {
  const canvas = canvasRef.value
  const host = canvas?.parentElement
  const context = canvas?.getContext('2d')
  if (!canvas || !host || !context) {
    console.error('LZ particle scene requires a 2D canvas context')
    return
  }

  const density = 2200
  const stars = Array.from({ length: 320 }, () => ({
    x: Math.random(), y: Math.random(), size: 0.35 + Math.random() * 1.35,
    phase: Math.random() * Math.PI * 2, speed: 0.0005 + Math.random() * 0.0015,
    orbit: 0.00003 + Math.random() * 0.00008,
  }))
  const points = Array.from({ length: density }, (_, index) => ({
    angle: (index / density) * Math.PI * 2, radius: 90 + Math.random() * 190,
    speed: 0.001 + Math.random() * 0.002, size: 0.55 + Math.random() * 1.7,
    x: 0, y: 0, tx: 0, ty: 0,
  }))
  const textTargets: Record<LzMode, { x: number; y: number }[]> = { AIFX: [], CC2CX: [], OPENAI: [], DEEPSEEK: [] }
  let lastMode: LzMode | null = null

  const createTextTargets = (mode: LzMode) => {
    const offscreen = document.createElement('canvas')
    offscreen.width = 1100
    offscreen.height = 300
    const textContext = offscreen.getContext('2d')
    if (!textContext) return
    textContext.clearRect(0, 0, offscreen.width, offscreen.height)
    textContext.fillStyle = '#fff'
    textContext.font = `800 ${mode.length > 6 ? 160 : 205}px Manrope`
    textContext.textAlign = 'center'
    textContext.textBaseline = 'middle'
    textContext.fillText(mode, 550, 150)
    const pixels = textContext.getImageData(0, 0, offscreen.width, offscreen.height).data
    const candidates: { x: number; y: number }[] = []
    for (let y = 8; y < offscreen.height - 8; y += 2) {
      for (let x = 8; x < offscreen.width - 8; x += 2) {
        if (pixels[(y * offscreen.width + x) * 4 + 3] > 120) candidates.push({ x: x - 550, y: y - 150 })
      }
    }
    textTargets[mode] = Array.from({ length: density }, (_, index) => candidates[(index * 29) % candidates.length] ?? { x: 0, y: 0 })
  }

  const resize = () => {
    const dpr = Math.min(window.devicePixelRatio || 1, 2)
    const width = host.clientWidth
    const height = host.clientHeight
    if (!width || !height) return
    canvas.width = width * dpr
    canvas.height = height * dpr
    canvas.style.width = `${width}px`
    canvas.style.height = `${height}px`
    context.setTransform(dpr, 0, 0, dpr, 0, 0)
    lastMode = null
  }
  resizeHandler = resize
  resize()
  window.addEventListener('resize', resize)

  const draw = (time: number) => {
    const width = host.clientWidth
    const height = host.clientHeight
    if (!width || !height) {
      frameId = requestAnimationFrame(draw)
      return
    }
    const centerX = width / 2
    const centerY = height * 0.51
    context.clearRect(0, 0, width, height)
    stars.forEach((star, index) => {
      const alpha = 0.18 + (Math.sin(time * star.speed + star.phase) + 1) * 0.2
      const baseX = (star.x - 0.5) * width
      const baseY = (star.y - 0.5) * height
      const angle = time * star.orbit + star.phase * 0.03
      const x = width * 0.5 + baseX * Math.cos(angle) - baseY * Math.sin(angle) + Math.sin(time * 0.00035 + star.phase) * 8
      const y = height * 0.5 + baseX * Math.sin(angle) + baseY * Math.cos(angle) + Math.cos(time * 0.00028 + star.phase) * 6
      context.beginPath()
      context.fillStyle = index % 9 === 0 ? '#a9d7ff' : '#dce7f5'
      context.globalAlpha = alpha
      context.arc(x, y, star.size, 0, Math.PI * 2)
      context.fill()
    })
    const palette: Record<LzMode, string> = { AIFX: '#86f3dc', CC2CX: '#bc9cff', OPENAI: '#86b9ff', DEEPSEEK: '#ffb87d' }
    const mode = props.activeMode
    if (lastMode !== mode) {
      createTextTargets(mode)
      const scale = Math.min(width / 820, 1.2)
      textTargets[mode].forEach((target, index) => {
        points[index].tx = centerX + target.x * scale
        points[index].ty = centerY + target.y * scale
        if (!points[index].x) {
          points[index].x = centerX + Math.cos(points[index].angle) * points[index].radius
          points[index].y = centerY + Math.sin(points[index].angle) * points[index].radius * 0.42
        }
      })
      lastMode = mode
    }
    points.forEach((point, index) => {
      point.angle += point.speed * (mode === 'CC2CX' ? 1.35 : 1)
      const drift = Math.sin(time * 0.0015 + index) * 1.7
      point.x += (point.tx + drift - point.x) * 0.045
      point.y += (point.ty + drift - point.y) * 0.045
      context.beginPath()
      context.fillStyle = palette[mode]
      context.globalAlpha = 0.32 + ((index * 7) % 10) / 18
      context.arc(point.x, point.y, point.size, 0, Math.PI * 2)
      context.fill()
    })
    context.globalAlpha = 1
    frameId = requestAnimationFrame(draw)
  }
  frameId = requestAnimationFrame(draw)
  cycleTimer = window.setInterval(() => emit('cycle'), 4200)
})

onBeforeUnmount(() => {
  cancelAnimationFrame(frameId)
  window.clearInterval(cycleTimer)
  if (resizeHandler) window.removeEventListener('resize', resizeHandler)
})
</script>

<style scoped>
.lz-particle-canvas {
  position: absolute;
  inset: 0;
  z-index: 0;
  display: block;
  height: 100%;
  width: 100%;
  pointer-events: none;
}
</style>
