<script setup lang="ts">
import { computed } from 'vue'
import { Chart as ChartJS, CategoryScale, LinearScale, PointElement, LineElement, Tooltip, Legend, Filler } from 'chart.js'
import { Line } from 'vue-chartjs'
import type { AgentAdminDailyPaymentStats } from '@/agent/api'

ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, Tooltip, Legend, Filler)

const props = defineProps<{ data: AgentAdminDailyPaymentStats[] }>()
const colors = [
  ['rgb(59, 130, 246)', 'rgba(59, 130, 246, 0.10)'],
  ['rgb(168, 85, 247)', 'rgba(168, 85, 247, 0.10)'],
  ['rgb(245, 158, 11)', 'rgba(245, 158, 11, 0.10)'],
  ['rgb(239, 68, 68)', 'rgba(239, 68, 68, 0.10)'],
]

const chartData = computed(() => {
  if (!props.data?.length) return null
  const currencies = [...new Set(props.data.flatMap(day => Object.keys(day.amount || {})))].sort()
  return {
    labels: props.data.map(day => day.date.slice(5)),
    datasets: [
      ...currencies.map((currency, index) => ({
        label: `${currency} 收入`,
        data: props.data.map(day => day.amount?.[currency] || 0),
        borderColor: colors[index % colors.length][0],
        backgroundColor: colors[index % colors.length][1],
        fill: true,
        tension: 0.3,
        pointRadius: props.data.length > 30 ? 0 : 2,
        pointHoverRadius: 5,
      })),
      {
        label: '订单数',
        data: props.data.map(day => day.count),
        borderColor: 'rgb(16, 185, 129)',
        backgroundColor: 'rgba(16, 185, 129, 0.10)',
        fill: false,
        tension: 0.3,
        pointRadius: props.data.length > 30 ? 0 : 2,
        pointHoverRadius: 5,
        yAxisID: 'orders',
      },
    ],
  }
})

const chartOptions = {
  responsive: true,
  maintainAspectRatio: false,
  interaction: { mode: 'index' as const, intersect: false },
  scales: {
    y: { type: 'linear' as const, position: 'left' as const, beginAtZero: true, title: { display: true, text: '支付金额' } },
    orders: { type: 'linear' as const, position: 'right' as const, beginAtZero: true, ticks: { precision: 0 }, grid: { drawOnChartArea: false }, title: { display: true, text: '订单数' } },
  },
  plugins: { legend: { position: 'top' as const } },
}
</script>

<template>
  <div class="card p-4">
    <h3 class="mb-4 text-sm font-semibold text-gray-900 dark:text-white">每日支付趋势</h3>
    <div class="h-64">
      <Line v-if="chartData" :data="chartData" :options="chartOptions" />
      <div v-else class="flex h-full items-center justify-center text-sm text-gray-500 dark:text-gray-400">暂无支付数据</div>
    </div>
  </div>
</template>
