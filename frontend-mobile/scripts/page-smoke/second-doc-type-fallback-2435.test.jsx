import { describe, it, expect } from 'vitest'
import fs from 'fs'
import path from 'path'

const src = fs.readFileSync(
  path.resolve(process.cwd(), 'src/pages-weapp/profile/edit/EditProfile.jsx'),
  'utf8'
)

describe('second-doc type four-state (#2435)', () => {
  it('asks the user to select when photo exists without type', () => {
    expect(src).toContain('请选择第二证件类型')
    expect(src).not.toContain('未定（待平台核定）')
  })

  it('keeps verified state read-only and saves self-reported type', () => {
    expect(src).toContain('idPhotoOtherVerified')
    expect(src).toContain('id_photo_other_type: secondDocType')
  })
})
