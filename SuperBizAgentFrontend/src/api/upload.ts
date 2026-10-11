import type { UploadRes } from '../types/api';
import { requestJson } from './http';

// uploadKnowledgeFile 上传文件到知识库，字段名需与后端 GetUploadFile("file") 一致。
export function uploadKnowledgeFile(file: File): Promise<UploadRes> {
  const formData = new FormData();
  formData.append('file', file);
  return requestJson<UploadRes>('/upload', {
    method: 'POST',
    body: formData,
  });
}
