export interface ApiConfig {
  baseUrl: string
  isAbsolute: boolean
}

/**
 * Returns the API base URL.
 * In SPA mode with Vite proxy, both dev and prod use relative URLs.
 * Set NUXT_PUBLIC_BACKEND_URL runtime config for custom backend location.
 */
export function getApiConfig(): ApiConfig {
  if (import.meta.server) {
    return { baseUrl: '', isAbsolute: false }
  }

  const config = useRuntimeConfig()
  const backendUrl = config.public.backendUrl as string

  if (backendUrl && backendUrl.trim() !== '') {
    return { baseUrl: backendUrl.replace(/\/$/, ''), isAbsolute: true }
  }

  return { baseUrl: '', isAbsolute: false }
}

export function buildApiUrl(endpoint: string): string {
  const config = getApiConfig()
  return config.baseUrl ? `${config.baseUrl}${endpoint}` : endpoint
}

export function buildSignalRUrl(hubPath: string): string {
  const config = getApiConfig()
  return config.baseUrl ? `${config.baseUrl}${hubPath}` : hubPath
}
