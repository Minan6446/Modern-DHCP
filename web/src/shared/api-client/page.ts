import type { ApiResponse, PageQuery, PageResult } from '@/types/system';

export const normalizePageResult = <T>(
  payload: unknown,
  params?: Partial<PageQuery>
): ApiResponse<PageResult<T>> => {
  const page = Number(params?.page ?? 1);
  const pageSize = Number(params?.pageSize ?? 10);

  if (payload && typeof payload === 'object' && 'code' in (payload as any) && 'data' in (payload as any)) {
    const envelope = payload as ApiResponse<any>;
    const data = envelope.data;
    if (Array.isArray(data)) {
      return { code: envelope.code ?? 0, message: envelope.message ?? '', data: { items: data as T[], total: data.length } };
    }
    if (data && typeof data === 'object') {
      if (Array.isArray((data as any).items)) {
        return {
          code: envelope.code ?? 0,
          message: envelope.message ?? '',
          data: {
            items: ((data as any).items || []) as T[],
            total: Number((data as any).total ?? ((data as any).items || []).length ?? 0)
          }
        };
      }
    }
  }

  const raw = (payload as any)?.data ?? payload;
  if (Array.isArray(raw)) {
    return { code: 0, message: '', data: { items: raw as T[], total: raw.length } };
  }
  if (raw && typeof raw === 'object' && Array.isArray((raw as any).items)) {
    return {
      code: 0,
      message: '',
      data: {
        items: ((raw as any).items || []) as T[],
        total: Number((raw as any).total ?? ((raw as any).items || []).length ?? 0)
      }
    };
  }

  return {
    code: 0,
    message: '',
    data: {
      items: [] as T[],
      total: 0,
      page,
      pageSize,
      hasMore: false
    } as PageResult<T>
  };
};
