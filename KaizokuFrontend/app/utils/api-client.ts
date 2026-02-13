import { getApiConfig } from './api-config'

class KaizokuApiClient {
  private resolveBaseUrl(): string {
    try {
      return getApiConfig().baseUrl
    } catch {
      return ''
    }
  }

  private async request<T>(
    endpoint: string,
    options: RequestInit = {},
  ): Promise<T> {
    const baseUrl = this.resolveBaseUrl()
    const url = baseUrl ? `${baseUrl}${endpoint}` : endpoint

    const isFormData = options.body instanceof FormData

    const response = await fetch(url, {
      headers: {
        ...(isFormData ? {} : { 'Content-Type': 'application/json' }),
        ...options.headers,
      },
      credentials: 'include',
      ...options,
    })

    if (!response.ok) {
      throw new Error(`API Error: ${response.status} ${response.statusText}`)
    }

    const contentLength = response.headers.get('content-length')
    const contentType = response.headers.get('content-type')

    if (contentLength === '0' || response.status === 204) {
      return undefined as T
    }

    const isJsonResponse = contentType?.includes('application/json')

    try {
      const text = await response.text()

      if (!text || text.trim() === '') {
        return undefined as T
      }

      if (isJsonResponse ?? (text.trim().startsWith('{') ?? text.trim().startsWith('['))) {
        try {
          const result = JSON.parse(text) as { data?: T } | T
          return result && typeof result === 'object' && 'data' in result && result.data !== undefined
            ? result.data
            : result as T
        }
        catch {
          if (text.trim() === '{}' || text.trim() === 'null') {
            return undefined as T
          }
          throw new Error(`Invalid JSON response: ${text}`)
        }
      }

      return text as T
    }
    catch (error) {
      if (error instanceof Error && error.message.includes('JSON')) {
        throw error
      }

      if (response.status === 200) {
        return undefined as T
      }

      throw new Error(`Failed to process response: ${error instanceof Error ? error.message : 'Unknown error'}`)
    }
  }

  async get<T>(endpoint: string): Promise<T> {
    return this.request<T>(endpoint, { method: 'GET' })
  }

  async post<T>(endpoint: string, data?: unknown): Promise<T> {
    return this.request<T>(endpoint, {
      method: 'POST',
      body: data instanceof FormData ? data : (data ? JSON.stringify(data) : undefined),
    })
  }

  async put<T>(endpoint: string, data?: unknown): Promise<T> {
    return this.request<T>(endpoint, {
      method: 'PUT',
      body: data ? JSON.stringify(data) : undefined,
    })
  }

  async patch<T>(endpoint: string, data?: unknown): Promise<T> {
    return this.request<T>(endpoint, {
      method: 'PATCH',
      body: data ? JSON.stringify(data) : undefined,
    })
  }

  async delete<T>(endpoint: string): Promise<T> {
    return this.request<T>(endpoint, { method: 'DELETE' })
  }
}

export const apiClient = new KaizokuApiClient()
