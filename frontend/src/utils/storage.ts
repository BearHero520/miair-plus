// Some embedded WebViews disallow persistent storage. Keep the current session usable.
const memory = new Map<string, string>()
export const storage = {
  getItem(key: string): string | null {
    try { return window.localStorage.getItem(key) ?? memory.get(key) ?? null }
    catch { return memory.get(key) ?? null }
  },
  setItem(key: string, value: string) {
    memory.set(key, value)
    try { window.localStorage.setItem(key, value) } catch { /* session only */ }
  },
  removeItem(key: string) {
    memory.delete(key)
    try { window.localStorage.removeItem(key) } catch { /* storage unavailable */ }
  },
}
