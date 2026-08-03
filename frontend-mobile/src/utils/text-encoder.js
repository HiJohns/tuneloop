// TextEncoder polyfill for WeChat mini-program environment.
// The qrcode library depends on TextEncoder (a browser API) which is not
// available in the mini-program JS runtime. This minimal UTF-8 encoder
// covers the ASCII/URL data the QR generator actually encodes.
const _TextEncoder = (() => {
  if (typeof globalThis !== 'undefined' && typeof globalThis.TextEncoder !== 'undefined') {
    return globalThis.TextEncoder
  }
  return class TextEncoder {
    encode(str) {
      const buf = []
      for (let i = 0; i < str.length; i++) {
        let code = str.charCodeAt(i)
        if (code < 0x80) {
          buf.push(code)
        } else if (code < 0x800) {
          buf.push(0xc0 | (code >> 6), 0x80 | (code & 0x3f))
        } else if (code < 0xd800 || code >= 0xe000) {
          buf.push(0xe0 | (code >> 12), 0x80 | ((code >> 6) & 0x3f), 0x80 | (code & 0x3f))
        } else {
          i++
          code = 0x10000 + (((code & 0x3ff) << 10) | (str.charCodeAt(i) & 0x3ff))
          buf.push(
            0xf0 | (code >> 18),
            0x80 | ((code >> 12) & 0x3f),
            0x80 | ((code >> 6) & 0x3f),
            0x80 | (code & 0x3f)
          )
        }
      }
      return new Uint8Array(buf)
    }
  }
})()

if (typeof globalThis !== 'undefined' && typeof globalThis.TextEncoder === 'undefined') {
  globalThis.TextEncoder = _TextEncoder
}
