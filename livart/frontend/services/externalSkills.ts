import type { ExternalSkillSummary } from '../types';
import { authHeaders } from './auth';
import { createApiError } from './errorMessages';

interface ApiResponse<T> {
  success: boolean;
  data?: T;
  error?: {
    message: string;
    code: string;
  };
}

export const loadExternalSkills = async (): Promise<ExternalSkillSummary[]> => {
  const response = await fetch('/api/skills', {
    headers: {
      Accept: 'application/json',
      ...authHeaders()
    }
  });
  const payload = await response.json().catch(() => null) as ApiResponse<ExternalSkillSummary[]> | null;
  if (!response.ok || !payload?.success) {
    throw createApiError(payload, response.status, '扩展工具列表暂时无法加载');
  }
  return Array.isArray(payload.data) ? payload.data : [];
};
