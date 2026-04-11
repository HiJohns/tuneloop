const isDebugMode = () => {
  if (typeof window === 'undefined') return false
  const urlParams = new URLSearchParams(window.location.search)
  return urlParams.get('debug') === 'true' || process.env.NODE_ENV === 'development'
}

const shouldLog = () => isDebugMode()

const Logger = {
  _getTimestamp() {
    return new Date().toISOString()
  },

  _formatCategory(category) {
    return `[${category}]`
  },

  log(category, message, ...args) {
    if (!shouldLog()) return
    console.log(this._getTimestamp(), this._formatCategory(category), message, ...args)
  },

  error(category, message, ...args) {
    if (!shouldLog()) return
    console.error(this._getTimestamp(), this._formatCategory(category), message, ...args)
  },

  warn(category, message, ...args) {
    if (!shouldLog()) return
    console.warn(this._getTimestamp(), this._formatCategory(category), message, ...args)
  },

  group(category, title) {
    if (!shouldLog()) return
    console.group(this._getTimestamp(), this._formatCategory(category), title)
  },

  groupEnd() {
    if (!shouldLog()) return
    console.groupEnd()
  },

  api(endpoint, method, data = null) {
    if (!shouldLog()) return
    this.group('API', `${method} ${endpoint}`)
    if (data) {
      console.log('Request/Response:', data)
    }
    this.groupEnd()
  },

  state(componentName, state) {
    if (!shouldLog()) return
    this.group('STATE', componentName)
    console.log('State Snapshot:', JSON.parse(JSON.stringify(state)))
    this.groupEnd()
  },

  ui(element, message, data = null) {
    if (!shouldLog()) return
    this.group('UI', element)
    console.log(message, data || '')
    this.groupEnd()
  }
}

export default Logger