// 负例夹具：写入 token 但未写 token_expiry（必须被扫描器检出）
export function persistToken(storage, token) {
  storage.setItem('token', token)
  return true
}
