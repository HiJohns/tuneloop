import base from './vitest.config.js'

export default {
  ...base,
  test: {
    ...base.test,
    include: ['scripts/page-smoke/checkout-defect.spec.jsx'],
  },
}
