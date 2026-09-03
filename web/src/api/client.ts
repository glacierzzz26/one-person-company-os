// /api/v1 信封 client:拆 {ok,data} / {ok:false,error{code,message}};Bearer token;
// 401(未授权)→ 触发全局 AuthModal;非 JSON / 网络失败统一成 ApiError(页面 toast 可读)。
const TOKEN_KEY = 'os.token';

export function getToken(): string {
  try {
    return localStorage.getItem(TOKEN_KEY) || '';
  } catch {
    return '';
  }
}

export function setToken(t: string): void {
  try {
    if (t) localStorage.setItem(TOKEN_KEY, t);
    else localStorage.removeItem(TOKEN_KEY);
  } catch {
    /* storage 不可用(隐私模式等)时静默,本会话仍可用 */
  }
}

export class ApiError extends Error {
  constructor(
    public status: number, // 0 = 网络层失败
    public code: string,
    message: string,
  ) {
    super(message);
    this.name = 'ApiError';
  }
}

// 全局未授权回调:由 AppContext 注册打开 AuthModal(避免 client 依赖 React 上下文)。
let unauthorizedHandler: (() => void) | null = null;
export function setUnauthorizedHandler(cb: (() => void) | null): void {
  unauthorizedHandler = cb;
}

type Envelope<T> = { ok: true; data: T } | { ok: false; error: { code: string; message: string } };

export async function request<T>(path: string, init: { method?: string; body?: unknown } = {}): Promise<T> {
  const headers: Record<string, string> = {};
  const tok = getToken();
  if (tok) headers['Authorization'] = `Bearer ${tok}`;
  if (init.body !== undefined) headers['Content-Type'] = 'application/json';

  let res: Response;
  try {
    res = await fetch(path, {
      method: init.method ?? 'GET',
      headers,
      body: init.body !== undefined ? JSON.stringify(init.body) : undefined,
    });
  } catch {
    throw new ApiError(0, 'network', `无法连接 os server(${path})——确认后端已起:os server --port 8787`);
  }

  // 401 → 弹 token 输入框(配了 OS_API_TOKEN 的后端;空后端不会 401)
  if (res.status === 401) {
    unauthorizedHandler?.();
    throw new ApiError(401, 'unauthorized', '未授权:请填写 OS_API_TOKEN(401)');
  }

  let payload: Envelope<T> | null = null;
  try {
    payload = (await res.json()) as Envelope<T>;
  } catch {
    payload = null;
  }

  if (payload && payload.ok) return payload.data;
  if (payload && !payload.ok) {
    throw new ApiError(res.status, payload.error.code, payload.error.message);
  }
  // 非 JSON(反向代理页等):透出状态码
  if (res.ok) throw new ApiError(res.status, 'bad_response', `非 JSON 响应(HTTP ${res.status})`);
  throw new ApiError(res.status, `http_${res.status}`, `HTTP ${res.status}`);
}
