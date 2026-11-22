// #2435 R3/R4：EditProfile 第二证件「四态渲染」+ 保存接线（真 render，非源码探针）。
// Audit 2026-11-22 要求：@testing-library/react + jsdom，断言 DOM 可见性/可操作性，
// 并捕获保存请求体；禁止 fs.readFileSync / grep 作为通过条件。
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { cleanup } from '@testing-library/react'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import { captureRender } from './harness'

const USER_WITH_PHOTO_TYPE = {
  name: '张三', nickname: 'zs', phone: '13800000000', email: 'a@b.c',
  id_photo_front: '/uploads/media/front.jpg',
  id_photo_back: '/uploads/media/back.jpg',
  id_photo_other: '/uploads/media/other.jpg',
  id_photo_other_type: 'student',
  id_photo_other_verified: false,
}

const USER_WITH_PHOTO_NO_TYPE = {
  ...USER_WITH_PHOTO_TYPE,
  id_photo_other_type: '',
}

const USER_NO_OTHER_PHOTO = {
  ...USER_WITH_PHOTO_TYPE,
  id_photo_other: '',
  id_photo_other_type: '',
}

const USER_VERIFIED = {
  ...USER_WITH_PHOTO_TYPE,
  id_photo_other_type: 'teacher',
  id_photo_other_verified: true,
}

const apiFetch = vi.fn()
const dialogToast = vi.fn()

vi.mock('../../src/services/api', () => ({
  apiFetch: (...args) => apiFetch(...args),
  getToken: () => 'test-token',
  resolveErrorMessage: (resp, fallback) => fallback,
}))

vi.mock('../../src/platform', () => ({
  env: { apiBaseUrl: 'http://api.test', isMiniProgram: false },
  dialog: { toast: (...args) => dialogToast(...args) },
  getInputValue: (e) => e?.target?.value ?? e?.detail?.value ?? '',
  storage: { get: () => null, set: () => {}, remove: () => {} },
  session: { get: () => null, set: () => {}, remove: () => {} },
  wxLogin: vi.fn(async () => ({})),
}))

vi.mock('../../src/platform/init', () => ({
  parseJWT: () => ({ role: 'USER', sub: 'user-1' }),
}))

vi.mock('../../src/components/AddressManager', () => ({ default: () => null }))
vi.mock('../../src/components/IntroLetterUploader', () => ({ default: () => null }))
vi.mock('../../src/components/IdPhotoUploader', () => ({
  default: (props) => <div data-testid={`uploader-${props.side}`} />,
}))
vi.mock('../../src/i18n', () => ({ t: (s) => s }))

async function renderEditProfile(user, waitMs = 400) {
  apiFetch.mockImplementation(async (url, options = {}) => {
    if ((options.method || 'GET') === 'GET' && String(url).includes('/users/me')) {
      return { json: async () => ({ code: 20000, data: user }) }
    }
    return { json: async () => ({ code: 20000, data: {} }) }
  })
  window.history.replaceState({}, '', '/profile/edit')
  const { default: EditProfile } = await import('../../src/pages-weapp/profile/edit/EditProfile')
  const errors = await captureRender(
    <MemoryRouter initialEntries={['/profile/edit']}>
      <Routes>
        <Route path="/profile/edit" element={<EditProfile />} />
      </Routes>
    </MemoryRouter>,
    waitMs
  )
  expect(errors, `fatal render errors:\n${errors.join('\n\n')}`).toEqual([])
  return document.body.textContent || ''
}

function saveButton() {
  return Array.from(document.querySelectorAll('button')).find((b) => b.textContent.trim() === '保存')
}

describe('EditProfile 第二证件四态渲染 (#2435 R3)', () => {
  beforeEach(() => {
    apiFetch.mockReset()
    dialogToast.mockReset()
  })
  afterEach(() => cleanup())

  it('R3-1: 有类型未审核 → 只读显示中文类型', async () => {
    const text = await renderEditProfile(USER_WITH_PHOTO_TYPE)
    expect(text).toContain('第二证件类型：学生证')
    expect(text).not.toContain('请选择第二证件类型')
    expect(text).toContain('可自行选择后保存；审核员可在审核时修正')
    process.stdout.write('R3_STATE_1_OK\n')
  })

  it('R3-2: 有照片无类型 → 提示选择，选择器可操作、保存可用', async () => {
    const text = await renderEditProfile(USER_WITH_PHOTO_NO_TYPE)
    expect(text).toContain('第二证件类型：请选择第二证件类型')
    const select = document.querySelector('select')
    expect(select, '类型选择器必须渲染').not.toBeNull()
    expect(select.disabled, '未审核时选择器必须可操作').toBe(false)
    const save = saveButton()
    expect(save, '保存按钮必须渲染').toBeTruthy()
    expect(save.disabled, '保存按钮必须可用').toBe(false)
    process.stdout.write('R3_STATE_2_OK\n')
  })

  it('R3-3: 无第二证件照片 → 状态行不渲染', async () => {
    const text = await renderEditProfile(USER_NO_OTHER_PHOTO)
    expect(text).not.toContain('第二证件类型：')
    expect(text).not.toContain('请选择第二证件类型')
    expect(text).not.toContain('已认证（类型由平台核定，不可自行修改）')
    // 未上传时仍保留上传入口（#2039 前置条件文案）
    expect(text).toContain('第二证件照（可选，请选择证件类型）')
    process.stdout.write('R3_STATE_3_OK\n')
  })

  it('R3-4: 已审核 → 中文类型 + 已认证只读', async () => {
    const text = await renderEditProfile(USER_VERIFIED)
    expect(text).toContain('第二证件类型：教职工证')
    expect(text).toContain('已认证（类型由平台核定，不可自行修改）')
    const select = document.querySelector('select')
    expect(select, '类型选择器必须渲染').not.toBeNull()
    expect(select.disabled, '已审核时选择器必须只读').toBe(true)
    process.stdout.write('R3_STATE_4_OK\n')
  })
})

describe('EditProfile 保存接线 (#2435 R4)', () => {
  beforeEach(() => {
    apiFetch.mockReset()
    dialogToast.mockReset()
  })
  afterEach(() => cleanup())

  async function saveAndCapture() {
    const save = saveButton()
    expect(save, '保存按钮必须渲染').toBeTruthy()
    save.click()
    await new Promise((resolve) => setTimeout(resolve, 300))
    const putCall = apiFetch.mock.calls.find(([, options]) => options && options.method === 'PUT')
    expect(putCall, '必须发起 PUT /users/me').toBeTruthy()
    return JSON.parse(putCall[1].body)
  }

  it('R4-1: 已存类型 → 请求体含 id_photo_other_type 与照片，状态一致', async () => {
    await renderEditProfile(USER_WITH_PHOTO_TYPE)
    const body = await saveAndCapture()
    expect(body.id_photo_other).toBe('/uploads/media/other.jpg')
    expect(body.id_photo_other_type).toBe('student')
    process.stdout.write('R4_BODY_WITH_TYPE_OK id_photo_other_type=' + body.id_photo_other_type + '\n')
  })

  it('R4-2: 有照片但类型为空 → 不误发类型字段', async () => {
    await renderEditProfile(USER_WITH_PHOTO_NO_TYPE)
    const body = await saveAndCapture()
    expect(body.id_photo_other).toBe('/uploads/media/other.jpg')
    expect(Object.prototype.hasOwnProperty.call(body, 'id_photo_other_type')).toBe(false)
    process.stdout.write('R4_NO_TYPE_OMITTED_OK\n')
  })

  it('R4-3: 无第二证件照片 → 照片与类型字段都不发送', async () => {
    await renderEditProfile(USER_NO_OTHER_PHOTO)
    const body = await saveAndCapture()
    expect(Object.prototype.hasOwnProperty.call(body, 'id_photo_other')).toBe(false)
    expect(Object.prototype.hasOwnProperty.call(body, 'id_photo_other_type')).toBe(false)
    process.stdout.write('R4_NO_PHOTO_OMITTED_OK\n')
  })
})
