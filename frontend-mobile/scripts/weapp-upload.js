#!/usr/bin/env node
const path = require('path')
const Module = require('module')

const requestPath = path.resolve(__dirname, '../node_modules/request/index.js')

const originalResolve = Module._resolveFilename
Module._resolveFilename = function (request, ...args) {
  if (request === 'request') return requestPath
  return originalResolve.call(this, request, ...args)
}

const req = require('request')
const originalReq = req.defaults ? req.defaults({}) : req

const patched = function (options, callback) {
  const opts = typeof options === 'string' ? { url: options } : { ...options }
  opts.family = 4
  return originalReq(opts, callback)
}
patched.defaults = originalReq.defaults.bind(originalReq)
patched.jar = originalReq.jar.bind(originalReq)
patched.cookie = originalReq.cookie.bind(originalReq)
patched.get = originalReq.get.bind(originalReq)
patched.post = originalReq.post.bind(originalReq)

delete require.cache[requestPath]
require.cache[requestPath] = { ...(require.cache[requestPath] || {}), exports: patched }

const args = process.argv.slice(2)
const { execSync } = require('child_process')
const cmd = [path.resolve(__dirname, '../node_modules/.bin/miniprogram-ci'), ...args].map(a => `"${a}"`).join(' ')
try {
  execSync(cmd, { stdio: 'inherit', cwd: path.resolve(__dirname, '..') })
} catch (e) {
  process.exit(e.status || 1)
}
