// #2527：批量重置密码响应 → 用户可见提示（纯函数，PC 无组件测试运行器，node --test 覆盖）。
//
// 背景：`POST /users/reset-password` 原先只返回 `sent`/`skipped`，归属缺失行被静默
// 丢弃后界面仍提示「已发送」。#2527 让后端把每一行显式分类（skipped_invisible /
// skipped_missing / skipped_no_email），前端必须消费这些分类，否则静默依旧存在（B7）。
//
// 返回 `{ level, text, sent, skipped, reasons }`：level 供调用方选择 message.success /
// message.warning；text 是给用户的完整文案（含被跳过数量与原因）。

const REASONS = [
  ['skipped_invisible', '个账号不在可管理范围'],
  ['skipped_missing', '个账号不存在或未同步 IAM 身份'],
  ['skipped_no_email', '个账号未配置邮箱'],
]

function arrayLength(value) {
  return Array.isArray(value) ? value.length : 0
}

export function resetPasswordSummary(data) {
  const payload = data || {}
  const sent = Number(payload.sent || 0)
  const skipped = Number(payload.skipped || 0)

  const reasons = []
  let skippedTotal = 0
  REASONS.forEach(([key, suffix]) => {
    const count = arrayLength(payload[key])
    if (count > 0) {
      skippedTotal += count
      reasons.push(`${count} ${suffix}`)
    }
  })

  // 旧响应（无分类字段）时退回后端 `skipped` 计数，避免提示「已发送」掩盖跳过。
  if (skippedTotal === 0 && skipped > 0) {
    skippedTotal = skipped
  }

  if (sent === 0) {
    return {
      level: 'warning',
      text: `未发送任何重设密码邮件：${reasons.length > 0 ? reasons.join('；') : '没有可处理的账号'}`,
      sent,
      skipped: skippedTotal,
      reasons,
    }
  }

  if (skippedTotal > 0) {
    return {
      level: 'warning',
      text: `已发送 ${sent} 封重设密码邮件；跳过 ${skippedTotal} 个（${reasons.length > 0 ? reasons.join('；') : '后端未发送'}）`,
      sent,
      skipped: skippedTotal,
      reasons,
    }
  }

  return {
    level: 'success',
    text: `已发送重设密码邮件（${sent} 个用户）`,
    sent,
    skipped: 0,
    reasons,
  }
}

export default resetPasswordSummary
