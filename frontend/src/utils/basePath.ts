// The server supplies the base for either fnOS gateway or direct-port access.
export const basePath = document.querySelector('base')?.getAttribute('href') || '/'
export const appURL = (path: string) => basePath + path.replace(/^\//, '')
