import React, { useState } from 'react';
import { Copy, Check } from 'lucide-react';
import { getFailureDetails } from '../services/errorMessages';

const FailureNotice = ({ error, context, compact = false }: { error: unknown; context?: string; compact?: boolean }) => {
  const failure = getFailureDetails(error, context);
  const [reason, ...suggestions] = failure.message.split('\n');
  const [copied, setCopied] = useState(false);
  const [copyFailed, setCopyFailed] = useState(false);
  const copyDetails = async () => {
    try {
      await navigator.clipboard.writeText([context, failure.message, failure.metadata, failure.details].filter(Boolean).join('\n'));
      setCopied(true);
      setCopyFailed(false);
    } catch {
      setCopyFailed(true);
    }
  };

  return (
    <div role="alert" className={`min-w-0 max-w-full text-left ${compact ? 'text-xs' : 'text-sm'} leading-6`}>
      {context && <p className="font-semibold text-rose-700">{context}未完成</p>}
      <p className="break-words font-medium text-rose-700">{reason}</p>
      {suggestions.length > 0 && <p className="mt-1 whitespace-pre-wrap break-words font-normal text-zinc-600">{suggestions.join('\n')}</p>}
      {failure.details && (
        <details className="mt-2 text-xs font-normal text-zinc-600" onPointerDown={event => event.stopPropagation()}>
          <summary className="cursor-pointer select-none text-zinc-500">查看排查详情</summary>
          <div className="mt-2 max-h-48 overflow-auto whitespace-pre-wrap break-all rounded border border-zinc-200 bg-white/80 p-2">
            {failure.metadata && <p className="mb-1 font-medium">{failure.metadata}</p>}
            <p>{failure.details}</p>
          </div>
          <button type="button" title="复制排查信息" aria-label="复制排查信息" onClick={copyDetails} className="mt-1 inline-flex h-8 items-center gap-1 px-1 hover:text-zinc-900">
            {copied ? <Check size={14} /> : <Copy size={14} />} {copied ? '已复制' : '复制排查信息'}
          </button>
          {copyFailed && <p>复制未完成，请选中上面的详情文字手动复制。</p>}
        </details>
      )}
    </div>
  );
};

export default FailureNotice;
