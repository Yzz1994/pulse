import { getToken, clearToken } from "./auth";

const BASE = "/v1";

export class AuthError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "AuthError";
  }
}

async function request<T>(path: string, opts?: RequestInit): Promise<T> {
  const token = getToken();
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
  };
  if (token) {
    headers["Authorization"] = `Bearer ${token}`;
  }

  const res = await fetch(`${BASE}${path}`, { ...opts, headers });

  // Auth redirect or 401 (but not the login endpoint itself)
  const isLoginEndpoint = path === "/auth/login";
  if (!isLoginEndpoint && (res.redirected || res.status === 401)) {
    clearToken();
    throw new AuthError("未登录，请先登录");
  }

  const ct = res.headers.get("content-type") ?? "";

  if (!res.ok) {
    if (ct.includes("application/json")) {
      const body = await res.json().catch(() => ({}));
      throw new Error(body.error ?? body.detail ?? `HTTP ${res.status}`);
    }
    // 非 JSON（如 Cloudflare 拦截了 5xx 并替换成 HTML），尝试提取纯文本
    const text = await res.text().catch(() => "");
    const plain = text.replace(/<[^>]*>/g, " ").replace(/\s+/g, " ").trim().slice(0, 120);
    throw new Error(plain || `HTTP ${res.status}`);
  }

  if (!ct.includes("application/json")) {
    throw new Error("服务端返回非 JSON 响应");
  }

  return res.json() as Promise<T>;
}

export const api = {
  get: <T>(path: string) => request<T>(path),
  post: <T>(path: string, body: unknown) =>
    request<T>(path, { method: "POST", body: JSON.stringify(body) }),
  put: <T>(path: string, body: unknown) =>
    request<T>(path, { method: "PUT", body: JSON.stringify(body) }),
  del: <T>(path: string) => request<T>(path, { method: "DELETE" }),
};

// ── CF Domain API ───────────────────────────────────────────────

import type {
  CFDomainsResponse,
  CFDomain,
  CFZonesResponse,
  CFDNSRecordsResponse,
  CFDNSRecord,
  CreateCFDomainRequest,
  CreateCFDNSRecordRequest,
  UpdateCFDNSRecordRequest,
  NodeDomainsResponse,
  NodeDomain,
  SyncNodeDomainsRequest,
  SyncNodeDomainsResponse,
  IXDomain,
  IXDomainsResponse,
} from "./types";

export const cfApi = {
  /** 获取已添加的域名列表 */
  listDomains: () => api.get<CFDomainsResponse>("/cf-domains"),

  /** 添加域名 */
  createDomain: (body: CreateCFDomainRequest) =>
    api.post<CFDomain>("/cf-domains", body),

  /** 删除域名 */
  deleteDomain: (id: string) => api.del(`/cf-domains/${id}`),

  /** 验证 Token 并返回可用 zones */
  verifyToken: (cfToken: string) =>
    api.post<CFZonesResponse>("/cf-domains/verify-token", { cf_token: cfToken }),

  /** 获取域名的 DNS 记录 */
  listRecords: (domainId: string, type?: string) => {
    const query = type ? `?type=${type}` : "";
    return api.get<CFDNSRecordsResponse>(`/cf-domains/${domainId}/records${query}`);
  },

  /** 添加 DNS 记录 */
  createRecord: (domainId: string, body: CreateCFDNSRecordRequest) =>
    api.post<CFDNSRecord>(`/cf-domains/${domainId}/records`, body),

  /** 更新 DNS 记录 */
  updateRecord: (domainId: string, recordId: string, body: UpdateCFDNSRecordRequest) =>
    api.put<CFDNSRecord>(`/cf-domains/${domainId}/records/${recordId}`, body),

  /** 删除 DNS 记录 */
  deleteRecord: (domainId: string, recordId: string) =>
    api.del(`/cf-domains/${domainId}/records/${recordId}`),
};

// ── IX Domain API ───────────────────────────────────────────────

export const ixApi = {
  list: () => api.get<IXDomainsResponse>("/ix-domains"),
  create: (body: Omit<IXDomain, "id">) => api.post<IXDomain>("/ix-domains", body),
  update: (id: string, body: Omit<IXDomain, "id">) => api.put<IXDomain>(`/ix-domains/${id}`, body),
  del: (id: string) => api.del(`/ix-domains/${id}`),
};

// ── Node Domain API ─────────────────────────────────────────────

export const nodeDomainApi = {
  /** 列出节点域名（支持按 cf_domain_id 过滤） */
  list: (cfDomainId?: string) => {
    const query = cfDomainId ? `?cf_domain_id=${cfDomainId}` : "";
    return api.get<NodeDomainsResponse>(`/node-domains${query}`);
  },

  /** 从 CF 同步 DNS 记录到本地库 */
  sync: (body: SyncNodeDomainsRequest) =>
    api.post<SyncNodeDomainsResponse>("/node-domains/sync", body),

  /** 更新节点分配 */
  updateNodeID: (id: string, nodeId: string) =>
    api.put<NodeDomain>(`/node-domains/${id}`, { node_id: nodeId }),

  /** 删除记录 */
  delete: (id: string) => api.del(`/node-domains/${id}`),
};
