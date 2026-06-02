export interface ApiResponse<T> {
  code: number
  data: T
  message: string
}

export type Dict = Record<string, unknown>

export interface IdResult {
  id: number | string
}

export interface SuccessResult {
  success: boolean
}
