// atob/btoa polyfill for WeChat mini-program (#1653).
// JSCore does not provide global atob/btoa; every atob(token.split('.')[1])
// JWT parse silently failed (getCartKey fell back to 'cart', cachePermissions
// got zeroed, role checks broke). Same injection pattern as utils/text-encoder.js.

var BASE64_CHARS = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/'

// base64DecodeToBytes decodes base64/base64url (JWT payload variant) to a byte array.
function base64DecodeToBytes(input) {
  var b64 = String(input).replace(/-/g, '+').replace(/_/g, '/')
  while (b64.length % 4) b64 += '='
  var bytes = []
  for (var i = 0; i < b64.length; i += 4) {
    var e1 = BASE64_CHARS.indexOf(b64[i])
    var e2 = BASE64_CHARS.indexOf(b64[i + 1])
    var e3 = BASE64_CHARS.indexOf(b64[i + 2])
    var e4 = BASE64_CHARS.indexOf(b64[i + 3])
    bytes.push((e1 << 2) | (e2 >> 4))
    if (e3 !== -1) bytes.push(((e2 & 15) << 4) | (e3 >> 2))
    if (e4 !== -1) bytes.push(((e3 & 3) << 6) | e4)
  }
  return bytes
}

// utf8Decode decodes a UTF-8 byte array to a string (JWT payload may contain
// CJK nicknames, so simple String.fromCharCode on raw bytes is wrong).
function utf8Decode(bytes) {
  var out = ''
  var i = 0
  while (i < bytes.length) {
    var c = bytes[i++]
    if (c < 0x80) {
      out += String.fromCharCode(c)
    } else if (c < 0xe0) {
      out += String.fromCharCode(((c & 0x1f) << 6) | (bytes[i++] & 0x3f))
    } else if (c < 0xf0) {
      out += String.fromCharCode(((c & 0x0f) << 12) | ((bytes[i++] & 0x3f) << 6) | (bytes[i++] & 0x3f))
    } else {
      var cp = ((c & 0x07) << 18) | ((bytes[i++] & 0x3f) << 12) | ((bytes[i++] & 0x3f) << 6) | (bytes[i++] & 0x3f)
      if (cp > 0xffff) {
        cp -= 0x10000
        out += String.fromCharCode(0xd800 + (cp >> 10), 0xdc00 + (cp & 0x3ff))
      } else {
        out += String.fromCharCode(cp)
      }
    }
  }
  return out
}

function atobImpl(input) {
  return utf8Decode(base64DecodeToBytes(input))
}

function btoaImpl(input) {
  var str = String(input)
  var bytes = []
  for (var i = 0; i < str.length; i++) {
    var code = str.charCodeAt(i)
    if (code < 0x80) {
      bytes.push(code)
    } else if (code < 0x800) {
      bytes.push(0xc0 | (code >> 6), 0x80 | (code & 0x3f))
    } else if (code < 0xd800 || code >= 0xe000) {
      bytes.push(0xe0 | (code >> 12), 0x80 | ((code >> 6) & 0x3f), 0x80 | (code & 0x3f))
    } else {
      i++
      code = 0x10000 + (((code & 0x3ff) << 10) | (str.charCodeAt(i) & 0x3ff))
      bytes.push(0xf0 | (code >> 18), 0x80 | ((code >> 12) & 0x3f), 0x80 | ((code >> 6) & 0x3f), 0x80 | (code & 0x3f))
    }
  }
  var out = ''
  for (var j = 0; j < bytes.length; j += 3) {
    var b1 = bytes[j]
    var b2 = j + 1 < bytes.length ? bytes[j + 1] : NaN
    var b3 = j + 2 < bytes.length ? bytes[j + 2] : NaN
    out += BASE64_CHARS[b1 >> 2]
    out += BASE64_CHARS[((b1 & 3) << 4) | ((b2 >> 4) & 15)]
    out += isNaN(b2) ? '=' : BASE64_CHARS[((b2 & 15) << 2) | ((b3 >> 6) & 3)]
    out += isNaN(b3) ? '=' : BASE64_CHARS[b3 & 63]
  }
  return out
}

var target = typeof globalThis !== 'undefined' ? globalThis : (typeof global !== 'undefined' ? global : (typeof window !== 'undefined' ? window : {}))
if (!target.atob) target.atob = atobImpl
if (!target.btoa) target.btoa = btoaImpl
