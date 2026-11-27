// #2615 丢失记录三页渲染验收（jsdom + @testing-library/react）。
//
// 覆盖（对应 Verification Matrix R2/R3/R4）：
//   R2 列表页：记录行信息 + 单一登记入口 + 行内无危险按钮 + 行点击进详情
//   R3 登记页：SN 检索选择器 + 校验（未选乐器 / 描述为空）+ 提交载荷 + 返回列表
//   R4 详情页：记录全文 + 照片 + 丢失中可恢复 + 已恢复隐藏恢复按钮 + 恢复载荷
//
// token 用 process.stdout.write（setup.js 屏蔽了 console.log），供
// scripts/verify-2615.sh 的 r2/r3/r4 断言。
import { describe, it, expect, vi, afterEach, beforeEach } from 'vitest'
import { render, waitFor, cleanup, fireEvent } from '@testing-library/react'
import { MemoryRouter, Routes, Route } from 'react-router-dom'

import InstrumentLossManage from '../../src/pages/InstrumentLossManage'
import InstrumentLossRegister from '../../src/pages/InstrumentLossRegister'
import InstrumentLossDetail from '../../src/pages/InstrumentLossDetail'
import { dialog } from '../../src/platform'

const token = (line) => process.stdout.write(`${line}\n`)

const jsonResponse = (body) => ({
  ok: true,
  status: 200,
  headers: { get: () => 'application/json' },
  json: async () => body,
  clone() { return this },
})

const lossRecords = [
  {
    id: 'rec-open-1', instrument_id: 'inst-1', responsible_party: 'user', user_ratio: 100,
    compensation_cents: 100000, user_burden_cents: 100000, description: '运输途中丢失',
    photos: ['k1', 'k2'], restored_at: null, restored_damaged: false,
    created_at: '2026-11-01T10:00:00Z',
    instrument_brief: { id: 'inst-1', sn: 'SN-2615-1', category_name: '吉他', level_name: '专业', cover_image: '', stock_status: 'lost' },
  },
  {
    id: 'rec-open-2', instrument_id: 'inst-2', responsible_party: 'logistics', user_ratio: 0,
    compensation_cents: 50000, user_burden_cents: 0, description: '物流丢件',
    photos: [], restored_at: null, restored_damaged: false,
    created_at: '2026-10-20T10:00:00Z',
    instrument_brief: { id: 'inst-2', sn: 'SN-2615-2', category_name: '贝斯', level_name: '', cover_image: '', stock_status: 'lost' },
  },
  {
    id: 'rec-done-1', instrument_id: 'inst-3', responsible_party: 'site', user_ratio: 0,
    compensation_cents: 20000, user_burden_cents: 0, description: '网点遗失后找回',
    photos: [], restored_at: '2026-10-02T09:00:00Z', restored_damaged: true,
    restore_description: '已找回，外壳有划痕', restore_photos: ['r1'],
    created_at: '2026-10-01T10:00:00Z',
    instrument_brief: { id: 'inst-3', sn: 'SN-2615-3', category_name: '提琴', level_name: '', cover_image: '', stock_status: 'available' },
  },
]

function stubFetch(handler) {
  const original = globalThis.fetch
  const calls = []
  globalThis.fetch = vi.fn(async (url, options = {}) => {
    calls.push({ url: String(url), options })
    return handler(String(url), options)
  })
  return { calls, restore: () => { globalThis.fetch = original } }
}

beforeEach(() => {
  vi.spyOn(dialog, 'alert').mockResolvedValue(undefined)
})

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

describe('instrument loss list (#2615 R2)', () => {
  it('renders record rows, single register entry and navigates to detail', async () => {
    const { calls, restore } = stubFetch((url) => {
      if (url.includes('/instrument-loss?') && url.includes('page=2')) {
        return jsonResponse({ code: 20000, data: { list: [], total: lossRecords.length, page: 2, page_size: 20 } })
      }
      if (url.includes('/instrument-loss?')) {
        return jsonResponse({ code: 20000, data: { list: lossRecords, total: lossRecords.length, page: 1, page_size: 20 } })
      }
      return jsonResponse({ code: 20000, data: { list: [], total: 0 } })
    })
    try {
      const { container } = render(
        <MemoryRouter initialEntries={['/instrument-loss-manage']}>
          <Routes>
            <Route path="/instrument-loss-manage" element={<InstrumentLossManage />} />
            <Route path="/instrument-loss-detail" element={<div data-testid="detail-route">detail</div>} />
            <Route path="/instrument-loss-register" element={<div data-testid="register-route">register</div>} />
          </Routes>
        </MemoryRouter>,
      )

      await waitFor(() => {
        expect(container.querySelectorAll('[data-testid="loss-row"]').length).toBe(3)
      })
      const rows = container.querySelectorAll('[data-testid="loss-row"]')
      token(`LOSS_LIST_ROWS=${rows.length}`)

      const bodyText = container.textContent
      token(`LOSS_LIST_SN_SHOWN=${bodyText.includes('SN: SN-2615-1') ? 1 : 0}`)
      token(`LOSS_LIST_TIME_SHOWN=${bodyText.includes('丢失于') ? 1 : 0}`)

      const badgeKinds = new Set()
      rows.forEach((row) => {
        if (row.textContent.includes('丢失中')) badgeKinds.add('open')
        if (row.textContent.includes('已恢复')) badgeKinds.add('restored')
      })
      token(`LOSS_LIST_STATUS_BADGE=${badgeKinds.size}`)

      // 行内不得出现任何按钮（危险动作只能在登记/详情页的确认按钮上）
      const dangerInRow = Array.from(rows).reduce((n, row) => n + row.querySelectorAll('button').length, 0)
      token(`LOSS_LIST_DANGER_BUTTON_IN_ROW=${dangerInRow}`)

      const entry = container.querySelector('[data-testid="loss-register-entry"]')
      token(`LOSS_LIST_REGISTER_ENTRY=${entry ? 1 : 0}`)

      // 单一入口 → 登记路由（先验证入口，再验证行点击，避免同一次渲染里互相干扰）
      fireEvent.click(entry)
      await waitFor(() => {
        expect(container.querySelector('[data-testid="register-route"]')).toBeTruthy()
      })
      cleanup()

      const second = render(
        <MemoryRouter initialEntries={['/instrument-loss-manage']}>
          <Routes>
            <Route path="/instrument-loss-manage" element={<InstrumentLossManage />} />
            <Route path="/instrument-loss-detail" element={<div data-testid="detail-route">detail</div>} />
          </Routes>
        </MemoryRouter>,
      )
      await waitFor(() => {
        expect(second.container.querySelectorAll('[data-testid="loss-row"]').length).toBe(3)
      })
      // 整行点击 → 详情路由（携带记录 id）
      fireEvent.click(second.container.querySelectorAll('[data-testid="loss-row"]')[0])
      await waitFor(() => {
        expect(second.container.querySelector('[data-testid="detail-route"]')).toBeTruthy()
      })
      token('LOSS_LIST_ROW_NAV=1')

      token('LOSS_LIST_RENDER_FAIL=0')
      expect(calls.some((c) => c.url.includes('/instrument-loss?'))).toBe(true)
    } finally {
      restore()
    }
  })
})

describe('instrument loss register (#2615 R3)', () => {
  it('searches instrument by SN, validates and submits the loss payload', async () => {
    const { calls, restore } = stubFetch((url) => {
      if (url.includes('/instruments?sn=')) {
        return jsonResponse({ code: 20000, data: { list: [{ id: 'inst-9', sn: 'SN-2615-9' }], total: 1 } })
      }
      if (url.includes('/instruments/inst-9/lost')) {
        return jsonResponse({ code: 20000, data: { id: 'rec-new' } })
      }
      return jsonResponse({ code: 20000, data: {} })
    })
    try {
      const { container } = render(
        <MemoryRouter initialEntries={['/manage-stub', '/instrument-loss-register']} initialIndex={1}>
          <Routes>
            <Route path="/manage-stub" element={<div data-testid="manage-stub">列表</div>} />
            <Route path="/instrument-loss-register" element={<InstrumentLossRegister />} />
          </Routes>
        </MemoryRouter>,
      )

      // 默认责任方 = 用户 + 比例 100%（现网语义）
      const ratioInput = container.querySelector('[data-testid="loss-ratio"]')
      const partyDefault = container.querySelector('[data-testid="loss-party-current"]')
      token(`LOSS_REG_PARTY_RATIO_DEFAULT=${ratioInput && ratioInput.value === '100' && partyDefault && partyDefault.textContent.includes('用户') ? 1 : 0}`)

      // 未选乐器直接提交 → 就地校验，不发请求
      fireEvent.click(container.querySelector('[data-testid="loss-submit"]'))
      await waitFor(() => expect(dialog.alert).toHaveBeenCalled())
      const noInstrumentAlert = dialog.alert.mock.calls.some((c) => String(c[0]).includes('请选择丢失的乐器'))
      const postedBeforeSelect = calls.some((c) => c.url.includes('/instruments/') && c.url.includes('/lost'))
      token(`LOSS_REG_VALIDATE_NO_INSTRUMENT=${noInstrumentAlert && !postedBeforeSelect ? 1 : 0}`)

      // SN 检索（防抖 300ms）→ 选项出现
      const snInput = container.querySelector('[data-testid="loss-inst-sn"]')
      fireEvent.input(snInput, { target: { value: 'SN-2615' } })
      await waitFor(() => {
        expect(container.querySelector('[data-testid="loss-inst-option"]')).toBeTruthy()
      }, { timeout: 3000 })
      token(`LOSS_REG_SELECTOR_QUERY=${calls.some((c) => c.url.includes('/instruments?sn=SN-2615')) ? 1 : 0}`)
      fireEvent.click(container.querySelector('[data-testid="loss-inst-option"]'))

      // 描述为空 → 校验（已选乐器后）
      dialog.alert.mockClear()
      fireEvent.click(container.querySelector('[data-testid="loss-submit"]'))
      await waitFor(() => expect(dialog.alert).toHaveBeenCalled())
      token(`LOSS_REG_VALIDATE_DESC_REQUIRED=${dialog.alert.mock.calls.some((c) => String(c[0]).includes('请填写丢失描述')) ? 1 : 0}`)

      // 正常提交 → 载荷字段齐全 + 返回列表
      const descInput = container.querySelector('[data-testid="loss-desc"]')
      fireEvent.input(descInput, { target: { value: '运输途中丢失' } })
      dialog.alert.mockClear()
      fireEvent.click(container.querySelector('[data-testid="loss-submit"]'))
      await waitFor(() => {
        expect(calls.some((c) => c.url.includes('/instruments/inst-9/lost'))).toBe(true)
      })
      const post = calls.find((c) => c.url.includes('/instruments/inst-9/lost'))
      const payload = JSON.parse(post.options.body)
      const payloadOK = payload.responsible_party === 'user'
        && payload.user_ratio === 100
        && payload.description === '运输途中丢失'
        && Array.isArray(payload.photos)
      token(`LOSS_REG_SUBMIT_PAYLOAD=${payloadOK ? 1 : 0}`)

      await waitFor(() => {
        expect(container.querySelector('[data-testid="manage-stub"]')).toBeTruthy()
      })
      token('LOSS_REG_SUBMIT_BACK=1')
      token('LOSS_REG_RENDER_FAIL=0')
    } finally {
      restore()
    }
  })
})

describe('instrument loss detail (#2615 R4)', () => {
  it('renders the open record with restore action and submits damage/photos', async () => {
    const { calls, restore } = stubFetch((url) => {
      if (url.includes('/instrument-loss/rec-open-1')) {
        return jsonResponse({ code: 20000, data: lossRecords[0] })
      }
      if (url.includes('/instruments/inst-1/restore')) {
        return jsonResponse({ code: 20000, data: { restored_damaged: true, action: 'put_back_on_shelf' } })
      }
      return jsonResponse({ code: 20000, data: {} })
    })
    try {
      const { container } = render(
        <MemoryRouter initialEntries={['/manage-stub', '/instrument-loss-detail?id=rec-open-1']} initialIndex={1}>
          <Routes>
            <Route path="/manage-stub" element={<div data-testid="manage-stub">列表</div>} />
            <Route path="/instrument-loss-detail" element={<InstrumentLossDetail />} />
          </Routes>
        </MemoryRouter>,
      )

      await waitFor(() => {
        expect(container.textContent).toContain('运输途中丢失')
      })
      const text = container.textContent
      const fieldsOK = text.includes('丢失时间')
        && text.includes('用户 · 100%')
        && text.includes('¥1000.00')
      token(`LOSS_DETAIL_FIELDS=${fieldsOK ? 1 : 0}`)

      const imgs = Array.from(container.querySelectorAll('img')).filter((i) => ['k1', 'k2'].includes(i.getAttribute('src')))
      token(`LOSS_DETAIL_PHOTOS=${imgs.length === 2 ? 1 : 0}`)

      const restoreBtn = container.querySelector('[data-testid="loss-open-restore"]')
      token(`LOSS_DETAIL_OPEN_HAS_RESTORE=${restoreBtn ? 1 : 0}`)

      fireEvent.click(restoreBtn)
      await waitFor(() => expect(container.querySelector('[data-testid="loss-submit-restore"]')).toBeTruthy())
      fireEvent.click(container.querySelector('[data-testid="loss-damaged-yes"]'))
      fireEvent.click(container.querySelector('[data-testid="loss-submit-restore"]'))
      await waitFor(() => {
        expect(calls.some((c) => c.url.includes('/instruments/inst-1/restore'))).toBe(true)
      })
      const post = calls.find((c) => c.url.includes('/instruments/inst-1/restore'))
      const payload = JSON.parse(post.options.body)
      token(`LOSS_DETAIL_RESTORE_PAYLOAD=${payload.damaged === true && Array.isArray(payload.photos) ? 1 : 0}`)
      token('LOSS_DETAIL_RENDER_FAIL=0')
    } finally {
      restore()
    }
  })

  it('hides the restore action for an already restored record', async () => {
    const { restore } = stubFetch((url) => {
      if (url.includes('/instrument-loss/rec-done-1')) {
        return jsonResponse({ code: 20000, data: lossRecords[2] })
      }
      return jsonResponse({ code: 20000, data: {} })
    })
    try {
      const { container } = render(
        <MemoryRouter initialEntries={['/instrument-loss-detail?id=rec-done-1']}>
          <Routes>
            <Route path="/instrument-loss-detail" element={<InstrumentLossDetail />} />
          </Routes>
        </MemoryRouter>,
      )
      await waitFor(() => {
        expect(container.textContent).toContain('恢复留痕')
      })
      const hasButton = container.querySelector('[data-testid="loss-open-restore"]')
      token(`LOSS_DETAIL_RESTORED_HIDES_RESTORE=${hasButton ? 0 : 1}`)
    } finally {
      restore()
    }
  })
})
