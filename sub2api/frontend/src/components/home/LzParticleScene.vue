<template>
  <canvas
    ref="canvasRef"
    :class="['lz-particle-canvas', `lz-particle-canvas--${variant}`]"
    aria-hidden="true"
  ></canvas>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'

export type LzMode = 'AIFX' | 'CC2CX' | 'OPENAI' | 'DEEPSEEK'
type ParticleVariant = 'focus' | 'ambient'

const props = withDefaults(
  defineProps<{
    activeMode: LzMode
    variant?: ParticleVariant
    autoCycle?: boolean
  }>(),
  {
    variant: 'focus',
    autoCycle: true,
  },
)
const emit = defineEmits<{ cycle: [] }>()
const canvasRef = ref<HTMLCanvasElement | null>(null)
let frameId = 0
let cycleTimer = 0
let resizeHandler: (() => void) | undefined
let resizeObserver: ResizeObserver | undefined
let pointerMoveHandler: ((event: PointerEvent) => void) | undefined

type AmbientParticle = {
  x: number
  y: number
  z: number
  size: number
  phase: number
  speed: number
  streak: boolean
  screenX: number
  screenY: number
}

type FocusParticle = {
  angle: number
  radius: number
  depth: number
  z: number
  speed: number
  size: number
  phase: number
  aura: boolean
  x: number
  y: number
  tx: number
  ty: number
  tz: number
}

type StarParticle = {
  x: number
  y: number
  z: number
  size: number
  phase: number
  speed: number
}

const palette: Record<LzMode, string> = {
  AIFX: '#7cf7d4',
  CC2CX: '#b99cff',
  OPENAI: '#78b7ff',
  DEEPSEEK: '#ffb36f',
}
const accentPalette: Record<LzMode, string> = {
  AIFX: '#d8fff5',
  CC2CX: '#f1e9ff',
  OPENAI: '#dcecff',
  DEEPSEEK: '#fff0dd',
}

const clamp01 = (value: number) => Math.max(0, Math.min(1, value))
const randomBetween = (min: number, max: number) => min + Math.random() * (max - min)

onMounted(() => {
  const canvas = canvasRef.value
  const host = canvas?.parentElement
  const context = canvas?.getContext('2d')
  if (!canvas || !host || !context) {
    console.error('LZ particle scene requires a 2D canvas context')
    return
  }

  const isAmbient = props.variant === 'ambient'
  let ambientParticles: AmbientParticle[] = []
  let ambientStars: StarParticle[] = []
  let ambientLinks: [number, number][] = []
  let ambientScreenX = new Float32Array(0)
  let ambientScreenY = new Float32Array(0)
  let ambientDepths = new Float32Array(0)

  let focusParticles: FocusParticle[] = []
  let focusStars: StarParticle[] = []
  let focusAuraLinks: [number, number][] = []
  let focusScreenX = new Float32Array(0)
  let focusScreenY = new Float32Array(0)
  let focusSizes = new Float32Array(0)
  let focusDepths = new Float32Array(0)
  let lastMode: LzMode | null = null
  let lastFrameTime = 0
  let pointerTargetX = 0
  let pointerTargetY = 0
  let pointerX = 0
  let pointerY = 0

  const createStars = (count: number): StarParticle[] =>
    Array.from({ length: count }, () => ({
      x: Math.random() * 2 - 1,
      y: Math.random() * 2 - 1,
      z: Math.random() * 2 - 1,
      size: randomBetween(0.35, 1.35),
      phase: Math.random() * Math.PI * 2,
      speed: randomBetween(0.0004, 0.0015),
    }))

  const initializeAmbient = (width: number, height: number) => {
    const area = Math.max(width * height, 390000)
    const count = Math.round(Math.min(1900, Math.max(980, area / 820)))
    ambientParticles = Array.from({ length: count }, () => ({
      x: Math.random() * 2 - 1,
      y: Math.random() * 2 - 1,
      z: Math.random() * 2 - 1,
      size: randomBetween(0.55, 2.55),
      phase: Math.random() * Math.PI * 2,
      speed: randomBetween(0.00028, 0.00108),
      streak: Math.random() < 0.16,
      screenX: -1,
      screenY: -1,
    }))
    ambientStars = createStars(Math.round(count * 0.22))
    const linkCount = Math.max(34, Math.round(width / 30))
    ambientLinks = Array.from({ length: linkCount }, () => [
      Math.floor(Math.random() * count),
      Math.floor(Math.random() * count),
    ])
    ambientScreenX = new Float32Array(count)
    ambientScreenY = new Float32Array(count)
    ambientDepths = new Float32Array(count)
  }

  const initializeFocus = (width: number, height: number) => {
    const centerX = width / 2
    const centerY = height * 0.51
    const count = Math.round(Math.min(2300, Math.max(1100, (width * height) / 260)))
    focusParticles = Array.from({ length: count }, (_, index) => {
      const angle = Math.random() * Math.PI * 2
      const aura = index % 9 === 0
      const radius = aura
        ? 0.5 + Math.pow(Math.random(), 0.72) * 0.5
        : 0.06 + Math.pow(Math.random(), 0.72) * 0.94
      const depth = Math.random()
      const z = Math.random() * 2 - 1
      const x = centerX + Math.cos(angle) * radius * width * 0.43
      const y = centerY + Math.sin(angle) * radius * height * 0.34
      return {
        angle,
        radius,
        depth,
        z,
        speed: randomBetween(0.55, 1.85),
        size: randomBetween(0.5, 2.05),
        phase: Math.random() * Math.PI * 2,
        aura,
        x,
        y,
        tx: x,
        ty: y,
        tz: z,
      }
    })
    focusStars = createStars(105)
    const auraIndices = focusParticles
      .map((particle, index) => (particle.aura ? index : -1))
      .filter((index) => index >= 0)
    focusAuraLinks = Array.from(
      { length: Math.min(34, Math.floor(auraIndices.length / 4)) },
      () => [
        auraIndices[Math.floor(Math.random() * auraIndices.length)],
        auraIndices[Math.floor(Math.random() * auraIndices.length)],
      ],
    )
    focusScreenX = new Float32Array(count)
    focusScreenY = new Float32Array(count)
    focusSizes = new Float32Array(count)
    focusDepths = new Float32Array(count)
    lastMode = null
  }

  const resize = () => {
    const width = host.clientWidth
    const height = host.clientHeight
    if (!width || !height) return
    const dpr = Math.min(window.devicePixelRatio || 1, 2)
    canvas.width = Math.max(1, Math.round(width * dpr))
    canvas.height = Math.max(1, Math.round(height * dpr))
    canvas.style.width = `${width}px`
    canvas.style.height = `${height}px`
    context.setTransform(dpr, 0, 0, dpr, 0, 0)
    if (isAmbient) {
      initializeAmbient(width, height)
    } else {
      initializeFocus(width, height)
    }
  }

  const drawStarField = (
    stars: StarParticle[],
    time: number,
    width: number,
    height: number,
    centerX: number,
    centerY: number,
    strength = 1,
  ) => {
    context.fillStyle = '#dcecff'
    stars.forEach((star) => {
      const perspective = 1 / (1 + star.z * 0.3)
      const x = centerX + star.x * width * 0.55 * perspective
      const y = centerY + star.y * height * 0.55 * perspective
      const twinkle = 0.5 + Math.sin(time * star.speed + star.phase) * 0.5
      context.globalAlpha = (0.09 + twinkle * 0.38) * strength
      context.beginPath()
      context.arc(x, y, star.size * (0.65 + perspective * 0.35), 0, Math.PI * 2)
      context.fill()
    })
  }

  const drawAmbient = (time: number, width: number, height: number) => {
    const mode = props.activeMode
    const color = palette[mode]
    const accent = accentPalette[mode]
    const centerX = width / 2
    const centerY = height / 2
    const yaw = time * 0.000085 + pointerX * 0.1
    const pitch = Math.sin(time * 0.00012) * 0.17 + pointerY * 0.065
    const cosYaw = Math.cos(yaw)
    const sinYaw = Math.sin(yaw)
    const cosPitch = Math.cos(pitch)
    const sinPitch = Math.sin(pitch)

    context.clearRect(0, 0, width, height)
    context.globalCompositeOperation = 'lighter'
    drawStarField(ambientStars, time, width, height, centerX, centerY, 0.9)

    ambientParticles.forEach((particle, index) => {
      const wobbleY = Math.sin(time * particle.speed + particle.phase) * 0.06
      const x1 = particle.x * cosYaw + particle.z * sinYaw
      const z1 = -particle.x * sinYaw + particle.z * cosYaw
      const y1 = particle.y * cosPitch - z1 * sinPitch + wobbleY
      const z2 =
        particle.y * sinPitch +
        z1 * cosPitch +
        Math.cos(time * particle.speed * 0.7 + particle.phase) * 0.05
      const perspective = 1 / (1 + z2 * 0.34)
      ambientScreenX[index] =
        centerX + x1 * width * 0.57 * perspective + Math.sin(time * 0.00042 + particle.phase) * 2.8
      ambientScreenY[index] =
        centerY + y1 * height * 0.57 * perspective + Math.cos(time * 0.00036 + particle.phase) * 2.4
      ambientDepths[index] = clamp01((perspective - 0.72) / 0.88)
    })

    context.strokeStyle = color
    context.lineWidth = 0.7
    context.globalAlpha = 0.085
    ambientLinks.forEach(([from, to]) => {
      context.beginPath()
      context.moveTo(ambientScreenX[from], ambientScreenY[from])
      context.lineTo(ambientScreenX[to], ambientScreenY[to])
      context.stroke()
    })

    context.fillStyle = color
    ambientParticles.forEach((particle, index) => {
      const depth = ambientDepths[index]
      const size = particle.size * (0.45 + depth * 1.1)
      const alpha = 0.05 + depth * 0.55
      if (particle.streak && particle.screenX >= 0) {
        context.globalAlpha = alpha * 0.2
        context.strokeStyle = color
        context.lineWidth = Math.max(0.45, size * 0.38)
        context.beginPath()
        context.moveTo(particle.screenX, particle.screenY)
        context.lineTo(ambientScreenX[index], ambientScreenY[index])
        context.stroke()
      }
      context.globalAlpha = alpha
      context.beginPath()
      context.arc(ambientScreenX[index], ambientScreenY[index], size, 0, Math.PI * 2)
      context.fill()
      particle.screenX = ambientScreenX[index]
      particle.screenY = ambientScreenY[index]
    })

    context.fillStyle = accent
    for (let index = 0; index < ambientParticles.length; index += 11) {
      const depth = ambientDepths[index]
      const radius = ambientParticles[index].size * (0.32 + depth * 0.48)
      if (depth > 0.55) {
        context.globalAlpha = 0.05 + depth * 0.16
        context.beginPath()
        context.arc(ambientScreenX[index], ambientScreenY[index], radius * 4.2, 0, Math.PI * 2)
        context.fill()
      }
      context.globalAlpha = 0.22 + depth * 0.68
      context.beginPath()
      context.arc(ambientScreenX[index], ambientScreenY[index], radius, 0, Math.PI * 2)
      context.fill()
    }

    context.globalAlpha = 1
    context.globalCompositeOperation = 'source-over'
  }

  const createTextTargets = (mode: LzMode, width: number) => {
    const offscreen = document.createElement('canvas')
    offscreen.width = 1100
    offscreen.height = 300
    const textContext = offscreen.getContext('2d')
    if (!textContext) return []
    textContext.clearRect(0, 0, offscreen.width, offscreen.height)
    textContext.fillStyle = '#fff'
    textContext.font = `800 ${mode.length > 6 ? 160 : 205}px Manrope, sans-serif`
    textContext.textAlign = 'center'
    textContext.textBaseline = 'middle'
    textContext.fillText(mode, 550, 150)
    const pixels = textContext.getImageData(0, 0, offscreen.width, offscreen.height).data
    const candidates: { x: number; y: number }[] = []
    for (let y = 8; y < offscreen.height - 8; y += 2) {
      for (let x = 8; x < offscreen.width - 8; x += 2) {
        if (pixels[(y * offscreen.width + x) * 4 + 3] > 120) {
          candidates.push({ x: x - 550, y: y - 150 })
        }
      }
    }
    if (!candidates.length) return []
    const scale = Math.min(width / 820, 1.2)
    const textParticleCount = focusParticles.reduce(
      (total, particle) => total + (particle.aura ? 0 : 1),
      0,
    )
    return Array.from({ length: textParticleCount }, (_, index) => {
      const candidate = candidates[(index * 37 + Math.floor(index / 19) * 11) % candidates.length]
      return { x: candidate.x * scale, y: candidate.y * scale }
    })
  }

  const assignTextTargets = (mode: LzMode, width: number, height: number) => {
    const targets = createTextTargets(mode, width)
    if (!targets.length) return
    const centerX = width / 2
    const centerY = height * 0.51
    let targetIndex = 0
    focusParticles.forEach((particle) => {
      if (particle.aura) return
      const target = targets[targetIndex % targets.length]
      particle.tx = centerX + target.x
      particle.ty = centerY + target.y
      particle.tz = -0.42 + particle.depth * 0.78
      targetIndex += 1
    })
    lastMode = mode
  }

  const drawFocus = (time: number, width: number, height: number, deltaMs: number) => {
    const mode = props.activeMode
    const color = palette[mode]
    const accent = accentPalette[mode]
    const centerX = width / 2
    const centerY = height * 0.51
    if (lastMode !== mode) assignTextTargets(mode, width, height)

    context.clearRect(0, 0, width, height)
    context.globalCompositeOperation = 'lighter'
    drawStarField(focusStars, time, width, height, centerX, centerY, 0.72)

    focusParticles.forEach((particle, index) => {
      if (particle.aura) {
        const orbit = particle.angle + time * 0.00022 * particle.speed
        particle.tx = centerX + Math.cos(orbit) * width * 0.44 * particle.radius
        particle.ty =
          centerY +
          Math.sin(orbit) * height * 0.35 * particle.radius +
          Math.sin(orbit * 0.7 + particle.phase) * height * 0.05
        particle.tz = Math.sin(orbit * 0.9 + particle.phase) * 0.9
      }
      const spring = particle.aura
        ? 1 - Math.exp(-deltaMs / 260)
        : 1 - Math.exp(-deltaMs / 70)
      particle.x += (particle.tx - particle.x) * spring
      particle.y += (particle.ty - particle.y) * spring
      const perspective = 1 + particle.tz * 0.17
      focusScreenX[index] = particle.x
      focusScreenY[index] = particle.y
      focusSizes[index] = particle.size * perspective * (particle.aura ? 1.18 : 1)
      focusDepths[index] = particle.depth
    })

    context.strokeStyle = color
    context.lineWidth = 0.55
    context.globalAlpha = 0.05
    focusAuraLinks.forEach(([from, to]) => {
      context.beginPath()
      context.moveTo(focusScreenX[from], focusScreenY[from])
      context.lineTo(focusScreenX[to], focusScreenY[to])
      context.stroke()
    })

    context.fillStyle = color
    focusParticles.forEach((particle, index) => {
      if (!particle.aura) return
      context.globalAlpha = 0.12 + focusDepths[index] * 0.3
      context.beginPath()
      context.arc(focusScreenX[index], focusScreenY[index], focusSizes[index] * 0.86, 0, Math.PI * 2)
      context.fill()
    })

    focusParticles.forEach((particle, index) => {
      if (particle.aura) return
      if (index % 23 === 0) {
        context.globalAlpha = 0.06 + focusDepths[index] * 0.08
        context.beginPath()
        context.arc(focusScreenX[index], focusScreenY[index], focusSizes[index] * 4, 0, Math.PI * 2)
        context.fill()
      }
      context.globalAlpha = 0.38 + focusDepths[index] * 0.52
      context.beginPath()
      context.arc(focusScreenX[index], focusScreenY[index], focusSizes[index], 0, Math.PI * 2)
      context.fill()
    })

    context.fillStyle = accent
    for (let index = 0; index < focusParticles.length; index += 13) {
      if (focusParticles[index].aura) continue
      context.globalAlpha = 0.5 + focusDepths[index] * 0.45
      context.beginPath()
      context.arc(focusScreenX[index], focusScreenY[index], focusSizes[index] * 0.54, 0, Math.PI * 2)
      context.fill()
    }

    context.globalAlpha = 1
    context.globalCompositeOperation = 'source-over'
  }

  const draw = (time: number) => {
    const width = host.clientWidth
    const height = host.clientHeight
    if (!width || !height) {
      frameId = requestAnimationFrame(draw)
      return
    }
    const deltaMs = lastFrameTime ? Math.min(120, Math.max(8, time - lastFrameTime)) : 16
    lastFrameTime = time
    const pointerEase = 1 - Math.exp(-deltaMs / 180)
    pointerX += (pointerTargetX - pointerX) * pointerEase
    pointerY += (pointerTargetY - pointerY) * pointerEase
    if (isAmbient) {
      drawAmbient(time, width, height)
    } else {
      drawFocus(time, width, height, deltaMs)
    }
    frameId = requestAnimationFrame(draw)
  }

  pointerMoveHandler = (event: PointerEvent) => {
    pointerTargetX = (event.clientX / Math.max(1, window.innerWidth)) * 2 - 1
    pointerTargetY = (event.clientY / Math.max(1, window.innerHeight)) * 2 - 1
  }

  resizeHandler = resize
  resize()
  window.addEventListener('resize', resize)
  window.addEventListener('pointermove', pointerMoveHandler, { passive: true })
  if (typeof ResizeObserver !== 'undefined') {
    resizeObserver = new ResizeObserver(resize)
    resizeObserver.observe(host)
  }
  frameId = requestAnimationFrame(draw)
  if (props.autoCycle) {
    cycleTimer = window.setInterval(() => emit('cycle'), 4200)
  }
})

onBeforeUnmount(() => {
  cancelAnimationFrame(frameId)
  window.clearInterval(cycleTimer)
  if (resizeHandler) window.removeEventListener('resize', resizeHandler)
  if (pointerMoveHandler) window.removeEventListener('pointermove', pointerMoveHandler)
  resizeObserver?.disconnect()
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

.lz-particle-canvas--ambient {
  z-index: 3;
  opacity: 0.9;
}

.lz-particle-canvas--focus {
  z-index: 0;
}
</style>
