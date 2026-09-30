// Chinese labels for copied, presentation-only Sub2API components.
// No main-site store, credentials or API client is imported here.
const labels: Record<string, string> = {
  'empty.noData': '暂无数据',
  'common.noData': '暂无数据',
  'common.selectAll': '全选',
  'common.selectOption': '选择',
  'common.search': '搜索',
  'common.searchPlaceholder': '搜索选项',
  'common.noOptionsFound': '没有匹配选项',
  'common.loading': '正在加载…',
}
export function useI18n() {
  return { t: (key: string) => labels[key] ?? key }
}
