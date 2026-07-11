export default function wxsCompat() {
  return {
    postcssPlugin: 'wxs-compat',
    Rule(rule) {
      rule.selector = rule.selector.split(',').map(s =>
        s.trim() === '*' ? 'page' : s.trim()
      ).join(', ')
      rule.selector = rule.selector.replace(/\\([!\/\[\]\(\)])/g, '$1')
    },
    Declaration(decl) {
      if (decl.value.includes('!important')) {
        decl.value = decl.value.replace(/\s*!important\s*/g, '')
      }
    },
  }
}
wxsCompat.postcss = true
