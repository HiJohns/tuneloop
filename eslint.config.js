import { defineConfig, globalIgnores } from 'eslint/config'

export default defineConfig([
  globalIgnores([
    '**/dist/**',
    '**/node_modules/**',
    '**/.git/**',
    'backend/**',
    'frontend-mobile-backup/**',
    'docs/**',
  ]),
])
