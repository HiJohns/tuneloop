const path = require('path')

const config = {
  projectName: 'tuneloop-mobile',
  date: '2026-6-11',
  designWidth: 750,
  deviceRatio: {
    640: 2.34 / 2,
    750: 1,
    828: 1.81 / 2,
  },
  sourceRoot: 'src',
  outputRoot: process.env.TARO_ENV === 'weapp' ? 'dist-weapp' : 'dist-h5',
  plugins: [
    '@tarojs/plugin-framework-react',
    '@tarojs/plugin-platform-weapp',
    '@tarojs/plugin-platform-h5',
  ],
  defineConstants: {},
  framework: 'react',
  compiler: 'webpack5',
  mini: {
    webpackChain(chain) {
      chain.resolve.alias
        .set('react-router-dom', path.resolve(__dirname, '../src/stubs/react-router-dom.js'))
        .set('lucide-react', path.resolve(__dirname, '../src/stubs/lucide-react.js'))
        .set('antd', path.resolve(__dirname, '../src/stubs/antd.js'))
        .set('@ant-design/icons', path.resolve(__dirname, '../src/stubs/@ant-design/icons.js'))

      chain.module
        .rule('esm-src')
        .test(/\.js$/)
        .include.add(path.resolve(__dirname, '../src'))
        .end()
        .type('javascript/esm')
    },
    postcss: {
      autoprefixer: { enable: true },
    },
  },
  h5: {
    publicPath: '/',
    staticDirectory: 'static',
    devServer: {
      port: 5553,
      host: '0.0.0.0',
    },
    postcss: {
      autoprefixer: { enable: true },
    },
  },
}

module.exports = function (merge) {
  if (process.env.NODE_ENV === 'development') {
    return merge({}, config, require('./dev'))
  }
  return merge({}, config, require('./prod'))
}
