import { Bookmark, Heart, Info, Sparkles, Tags } from 'lucide-react'

import { getCell } from '../domain/cellCatalog.js'
import { getAssetMetadata } from '../lib/assetMetadata.js'
import { t } from '../i18n.js'

const FACT_LABELS = {
  Category: 'category',
  Source: 'sourceLabel',
  Provider: 'provider',
  Status: 'status',
  Scene: 'scene',
  Analyzer: 'analyzer',
  Scale: 'scale',
  Task: 'task',
}

const FACT_VALUE_LABELS = {
  'Built-in': '内置场景',
  'Built-in starter scene': '内置示例场景',
  'Interactive starter': '可交互示例',
  'Local rules': '本地规则',
  'Reference ready': '参考模型就绪',
  'GLB ready': 'GLB 已就绪',
  'Generation failed': '生成失败',
  Queued: '排队中',
  processing: '生成中',
  running: '生成中',
  success: '已完成',
  'Khronos Reference': 'Khronos 参考',
  'Local GLB import': '本地 GLB 导入',
  'Uploaded reference image': '上传的参考图',
  'Generated workspace asset': '工作区生成资产',
  'Specimen Lab': '标本实验室',
}

export function DetailPanel({ selectedCell, favoriteKey, setFavoriteKey, customCells, onNotify, language = 'zh' }) {
  const cell = getCell(selectedCell, customCells)
  const metadata = getAssetMetadata(cell)
  const currentKey = `${selectedCell}:asset`
  const isFavorite = favoriteKey === currentKey

  function toggleFavorite() {
    const next = isFavorite ? '' : currentKey
    setFavoriteKey(next)
    onNotify(isFavorite ? `${metadata.title} ${t(language, 'favoriteRemoved')}` : `${metadata.title} ${t(language, 'favoriteSaved')}`)
  }

  return (
    <aside className="right-rail" aria-label={t(language, 'assetDetails')}>
      <section className="panel detail-panel">
        <header className="detail-title">
          <span>
            <Info size={14} />
            {t(language, 'assetDetails')}
          </span>
          <button type="button" className={isFavorite ? 'detail-fav active' : 'detail-fav'} onClick={toggleFavorite} aria-pressed={isFavorite}>
            <Heart size={15} fill={isFavorite ? 'currentColor' : 'none'} />
          </button>
        </header>
        <div className="detail-section-label">{t(language, 'overview')}</div>
        <div className="detail-heading asset-heading">
          <div className="cluster-icon asset-icon" style={{ '--cluster': metadata.accent }}>
            <span />
            <span />
            <span />
            <span />
          </div>
          <div>
            <h2>{metadata.title}</h2>
            <p>{metadata.subtitle}</p>
          </div>
        </div>
        <div className="detail-section-label">{t(language, 'technicalInfo')}</div>
        <dl className="detail-grid">
          {metadata.facts.map(([label, value]) => (
            <div key={label}>
              <dt>{FACT_LABELS[label] ? t(language, FACT_LABELS[label]) : label}</dt>
              <dd>{FACT_VALUE_LABELS[value] || value}</dd>
            </div>
          ))}
        </dl>
      </section>

      <section className="panel notes-panel">
        <header className="panel-title">
          <span>
            <Bookmark size={14} />
            {t(language, 'objectDescription')}
          </span>
        </header>
        <p>{metadata.description}</p>
        <blockquote>
          <strong>{t(language, 'systemAnalysis')}</strong>
          <span>{metadata.value}</span>
        </blockquote>
      </section>

      <section className="panel asset-tags-panel">
        <header className="panel-title">
          <span>
            <Tags size={14} />
            {t(language, 'tags')}
          </span>
        </header>
        <div className="asset-tag-list">
          {metadata.tags.map((tag) => (
            <span key={tag}>{tag}</span>
          ))}
        </div>
        <p>
          <Sparkles size={13} />
          {t(language, 'inferredBy')}：{metadata.insightSource}
        </p>
      </section>
    </aside>
  )
}
