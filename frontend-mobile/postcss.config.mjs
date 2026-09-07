import tailwindcss from 'tailwindcss'
import autoprefixer from 'autoprefixer'

const isWeapp = process.env.TARO_ENV === 'weapp'

const wxsCompat = {
  postcssPlugin: 'wxs-compat',
  Rule(rule) {
    if (!isWeapp) return
    if (rule.selector.includes('[')) {
      rule.remove()
      return
    }
    rule.selector = rule.selector.replace(/\\([!])/g, '')
    rule.selector = rule.selector.replace(/\\([\[\]\(\)\.])/g, '$1')
    if (rule.selector.includes('\\')) {
      rule.remove()
      return
    }
  },
  Declaration(decl) {
    if (!isWeapp) return
    if (decl.important) {
      decl.important = false
    }
  },
}

export default {
  plugins: [tailwindcss, wxsCompat, autoprefixer],
}
