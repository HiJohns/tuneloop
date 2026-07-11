import tailwindcss from 'tailwindcss'
import autoprefixer from 'autoprefixer'

const wxsCompat = {
  postcssPlugin: 'wxs-compat',
  Rule(rule) {
    if (rule.selector.includes('[')) {
      rule.remove()
      return
    }
    rule.selector = rule.selector.replace(/\\([!])/g, '')
    rule.selector = rule.selector.replace(/\\([\[\]\(\)])/g, '$1')
  },
  Declaration(decl) {
    if (decl.value.includes('!important')) {
      decl.value = decl.value.replace(/\s*!important\s*/g, '')
    }
  },
}

export default {
  plugins: [tailwindcss, wxsCompat, autoprefixer],
}
