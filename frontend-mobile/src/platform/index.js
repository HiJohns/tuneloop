const platform = (typeof process !== 'undefined' && process.env.TARO_ENV === 'weapp')
  ? require('./index.weapp')
  : require('./browser')

export const {
  storage,
  session,
  cookie,
  request,
  uploadFile,
  dialog,
  navigation,
  env,
} = platform
