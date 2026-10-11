import type { AIOpsRes } from '../types/api';
import { requestJson } from './http';

// requestAIOps 触发 AI 运维分析。
export function requestAIOps(): Promise<AIOpsRes> {
  return requestJson<AIOpsRes>('/ai_ops', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
  });
}
